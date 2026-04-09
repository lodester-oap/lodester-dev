// SPDX-License-Identifier: AGPL-3.0-or-later

// Lodester client — zero-knowledge auth
// Master password NEVER leaves the browser.
// Flow: master_password → Argon2id(salt=email) → login_hash → server

"use strict";

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
const API_BASE = "/api/v1";

// KDF parameters — must match DECISION-045
const KDF_PARAMS = {
  algorithm: "argon2id",
  memory: 65536,   // 64 MB
  iterations: 3,
  parallelism: 4,
  hashLength: 32,
};

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
let sessionState = {
  token: null,
  userId: null,
  email: null,
  encryptionKey: null, // CryptoKey (AES-GCM-256), derived client-side
  vaultVersion: 0,     // current vault version for optimistic locking
};

// ---------------------------------------------------------------------------
// Argon2id (WASM)
// ---------------------------------------------------------------------------
// argon2-browser v1.18.0 is loaded from CDN with SRI (Subresource Integrity).
// The integrity hash prevents a compromised CDN from serving malicious code.
// Phase 1b will bundle this library locally via go:embed.
// See docs/client-crypto-implementation.md for details.
let argon2Ready = false;
let argon2Module = null;

const ARGON2_SCRIPT_URL = "https://cdn.jsdelivr.net/npm/argon2-browser@1.18.0/dist/argon2-bundled.min.js";
// SHA-384 hash of argon2-bundled.min.js v1.18.0 (verified 2026-04-09)
const ARGON2_SCRIPT_SRI = "sha384-XOR3aNvHciLPIf6r+2glkrmbBbLmIJ1EChMXjw8eBKBf8gE0rDq1TyUNuRdorOqi";

function loadArgon2() {
  return new Promise((resolve, reject) => {
    if (argon2Ready) { resolve(); return; }
    const script = document.createElement("script");
    script.src = ARGON2_SCRIPT_URL;
    script.integrity = ARGON2_SCRIPT_SRI;
    script.crossOrigin = "anonymous";
    script.onload = () => {
      if (typeof argon2 !== "undefined") {
        argon2Module = argon2;
        argon2Ready = true;
        resolve();
      } else {
        reject(new Error("argon2 module not found after script load"));
      }
    };
    script.onerror = () => reject(new Error("Failed to load argon2-browser (SRI mismatch or network error)"));
    document.head.appendChild(script);
  });
}

// Derive login_hash: Argon2id(password=masterPassword, salt=normalizedEmail)
// Returns hex-encoded 32-byte hash.
async function deriveLoginHash(email, masterPassword) {
  await loadArgon2();

  // Normalize email: domain lowercase, local part preserved (DECISION-046)
  const normalized = normalizeEmail(email);

  // Use email as salt (encoded as UTF-8)
  const encoder = new TextEncoder();
  const salt = encoder.encode(normalized);
  const pass = encoder.encode(masterPassword);

  const result = await argon2Module.hash({
    pass: pass,
    salt: salt,
    time: KDF_PARAMS.iterations,
    mem: KDF_PARAMS.memory,
    parallelism: KDF_PARAMS.parallelism,
    hashLen: KDF_PARAMS.hashLength,
    type: argon2Module.ArgonType.Argon2id,
  });

  // result.hashHex is the hex-encoded hash
  return result.hashHex;
}

function normalizeEmail(email) {
  const at = email.indexOf("@");
  if (at === -1) return email;
  return email.substring(0, at) + "@" + email.substring(at + 1).toLowerCase();
}

// ---------------------------------------------------------------------------
// Encryption (AES-GCM-256 via Web Crypto API)
// ---------------------------------------------------------------------------
// Derive encryption key: Argon2id(password, salt=email) → masterKey → HKDF → encKey
// The masterKey is the same Argon2id output used for login_hash derivation.
// We use HKDF-SHA256 with info="lodester-encryption" to derive a separate key.

async function deriveEncryptionKey(email, masterPassword) {
  await loadArgon2();
  const normalized = normalizeEmail(email);
  const encoder = new TextEncoder();

  const result = await argon2Module.hash({
    pass: encoder.encode(masterPassword),
    salt: encoder.encode(normalized),
    time: KDF_PARAMS.iterations,
    mem: KDF_PARAMS.memory,
    parallelism: KDF_PARAMS.parallelism,
    hashLen: KDF_PARAMS.hashLength,
    type: argon2Module.ArgonType.Argon2id,
  });

  // Import the raw Argon2id hash as HKDF key material
  const masterKey = await crypto.subtle.importKey(
    "raw", result.hash, { name: "HKDF" }, false, ["deriveKey"]
  );

  // Derive AES-GCM-256 key via HKDF-SHA256
  const encKey = await crypto.subtle.deriveKey(
    {
      name: "HKDF",
      hash: "SHA-256",
      salt: encoder.encode(normalized),
      info: encoder.encode("lodester-encryption"),
    },
    masterKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"]
  );

  return encKey;
}

// Encrypt plaintext → vault blob (header + ciphertext).
// Wire format: [4-byte headerLen BE] [header JSON] [AES-GCM ciphertext]
// CRITICAL: nonce is generated fresh every time via crypto.getRandomValues.
async function encryptVaultData(plaintext, encKey) {
  const encoder = new TextEncoder();
  const data = encoder.encode(JSON.stringify(plaintext));

  // Generate a fresh 12-byte nonce (NEVER reuse!)
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);

  const ciphertext = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv: nonce },
    encKey,
    data
  );

  const nonceB64 = uint8ToBase64url(nonce);
  const header = {
    v: 1,
    alg: "aes-gcm-256",
    kdf: KDF_PARAMS.algorithm,
    kdf_params: {
      memory: KDF_PARAMS.memory,
      iterations: KDF_PARAMS.iterations,
      parallelism: KDF_PARAMS.parallelism,
    },
    nonce: nonceB64,
    ct_len: ciphertext.byteLength,
  };

  const headerJSON = encoder.encode(JSON.stringify(header));
  const headerLen = new Uint8Array(4);
  new DataView(headerLen.buffer).setUint32(0, headerJSON.length, false); // big-endian

  // Assemble blob: [headerLen][headerJSON][ciphertext]
  const blob = new Uint8Array(4 + headerJSON.length + ciphertext.byteLength);
  blob.set(headerLen, 0);
  blob.set(headerJSON, 4);
  blob.set(new Uint8Array(ciphertext), 4 + headerJSON.length);

  return blob;
}

// Decrypt vault blob → plaintext object.
async function decryptVaultData(blobBytes, encKey) {
  if (blobBytes.length < 4) throw new Error("vault blob too short");

  const headerLen = new DataView(blobBytes.buffer, blobBytes.byteOffset, 4).getUint32(0, false);
  if (blobBytes.length < 4 + headerLen) throw new Error("invalid header length");

  const decoder = new TextDecoder();
  const header = JSON.parse(decoder.decode(blobBytes.slice(4, 4 + headerLen)));
  const ciphertext = blobBytes.slice(4 + headerLen);

  const nonce = base64urlToUint8(header.nonce);

  const plainBytes = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: nonce },
    encKey,
    ciphertext
  );

  return JSON.parse(decoder.decode(plainBytes));
}

function uint8ToBase64url(arr) {
  let binary = "";
  for (let i = 0; i < arr.length; i++) binary += String.fromCharCode(arr[i]);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function base64urlToUint8(str) {
  str = str.replace(/-/g, "+").replace(/_/g, "/");
  while (str.length % 4) str += "=";
  const binary = atob(str);
  const arr = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) arr[i] = binary.charCodeAt(i);
  return arr;
}

// ---------------------------------------------------------------------------
// API calls
// ---------------------------------------------------------------------------
async function apiPost(path, body) {
  const headers = { "Content-Type": "application/json" };
  if (sessionState.token) {
    headers["Authorization"] = "Bearer " + sessionState.token;
  }
  const resp = await fetch(API_BASE + path, {
    method: "POST",
    headers: headers,
    body: JSON.stringify(body),
  });
  const data = await resp.json();
  return { status: resp.status, data: data };
}

async function apiGet(path) {
  const headers = {};
  if (sessionState.token) {
    headers["Authorization"] = "Bearer " + sessionState.token;
  }
  const resp = await fetch(API_BASE + path, { headers: headers });
  const data = await resp.json();
  return { status: resp.status, data: data };
}

async function apiPut(path, body) {
  const headers = { "Content-Type": "application/json" };
  if (sessionState.token) {
    headers["Authorization"] = "Bearer " + sessionState.token;
  }
  const resp = await fetch(API_BASE + path, {
    method: "PUT",
    headers: headers,
    body: JSON.stringify(body),
  });
  const data = await resp.json();
  return { status: resp.status, data: data };
}

async function apiDelete(path) {
  const headers = {};
  if (sessionState.token) {
    headers["Authorization"] = "Bearer " + sessionState.token;
  }
  const resp = await fetch(API_BASE + path, { method: "DELETE", headers: headers });
  if (resp.status === 204) return { status: 204, data: null };
  const data = await resp.json().catch(() => null);
  return { status: resp.status, data: data };
}

// apiPostRaw returns the raw Response so callers can stream the body
// (used for vCard file downloads).
async function apiPostRaw(path, body) {
  const headers = { "Content-Type": "application/json" };
  if (sessionState.token) {
    headers["Authorization"] = "Bearer " + sessionState.token;
  }
  return fetch(API_BASE + path, {
    method: "POST",
    headers: headers,
    body: JSON.stringify(body),
  });
}

// ---------------------------------------------------------------------------
// UI helpers
// ---------------------------------------------------------------------------
function showTab(tab) {
  document.getElementById("form-login").classList.toggle("hidden", tab !== "login");
  document.getElementById("form-register").classList.toggle("hidden", tab !== "register");
  document.getElementById("tab-login").classList.toggle("active", tab === "login");
  document.getElementById("tab-register").classList.toggle("active", tab === "register");
  clearMessages();
}

function showMessage(elementId, text, type) {
  const el = document.getElementById(elementId);
  el.className = "msg " + type;
  el.textContent = text;
}

function clearMessages() {
  document.querySelectorAll(".msg").forEach(el => {
    el.className = "msg";
    el.textContent = "";
  });
}

function setLoading(buttonId, loading) {
  const btn = document.getElementById(buttonId);
  btn.disabled = loading;
  if (loading) {
    btn.dataset.originalText = btn.textContent;
    btn.textContent = "処理中...";
  } else {
    btn.textContent = btn.dataset.originalText || btn.textContent;
  }
}

function showDashboard() {
  document.getElementById("auth-section").classList.add("hidden");
  document.getElementById("dashboard-section").classList.remove("hidden");
  document.getElementById("dash-user-id").textContent = sessionState.userId || "-";
  document.getElementById("dash-email").textContent = sessionState.email || "-";
}

function showAuth() {
  document.getElementById("auth-section").classList.remove("hidden");
  document.getElementById("dashboard-section").classList.add("hidden");
  showTab("login");
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------
async function handleRegister(event) {
  event.preventDefault();
  clearMessages();

  const email = document.getElementById("reg-email").value.trim();
  const password = document.getElementById("reg-password").value;
  const confirm = document.getElementById("reg-password-confirm").value;

  if (password !== confirm) {
    showMessage("reg-msg", "パスワードが一致しません。", "error");
    return;
  }

  setLoading("reg-btn", true);
  showMessage("reg-msg", "鍵を導出中... (数秒かかります)", "info");

  try {
    const loginHash = await deriveLoginHash(email, password);
    showMessage("reg-msg", "アカウントを作成中...", "info");

    const resp = await apiPost("/accounts", {
      email: email,
      login_hash: loginHash,
      kdf_params: {
        algorithm: KDF_PARAMS.algorithm,
        memory: KDF_PARAMS.memory,
        iterations: KDF_PARAMS.iterations,
        parallelism: KDF_PARAMS.parallelism,
      },
    });

    if (resp.status === 201) {
      showMessage("reg-msg", "アカウントが作成されました。ログインしてください。", "success");
      // Switch to login tab after short delay
      setTimeout(() => {
        showTab("login");
        document.getElementById("login-email").value = email;
      }, 1500);
    } else if (resp.status === 409) {
      showMessage("reg-msg", "このメールアドレスは既に登録されています。", "error");
    } else {
      const msg = resp.data?.error?.message || "登録に失敗しました。";
      showMessage("reg-msg", msg, "error");
    }
  } catch (err) {
    showMessage("reg-msg", "エラー: " + err.message, "error");
  } finally {
    setLoading("reg-btn", false);
  }
}

async function handleLogin(event) {
  event.preventDefault();
  clearMessages();

  const email = document.getElementById("login-email").value.trim();
  const password = document.getElementById("login-password").value;

  setLoading("login-btn", true);
  showMessage("login-msg", "鍵を導出中... (数秒かかります)", "info");

  try {
    const loginHash = await deriveLoginHash(email, password);
    showMessage("login-msg", "認証中...", "info");

    const resp = await apiPost("/sessions", {
      email: email,
      login_hash: loginHash,
    });

    if (resp.status === 200) {
      sessionState.token = resp.data.token;
      sessionState.userId = resp.data.user_id;
      sessionState.email = email;

      // Derive encryption key (same master password, separate HKDF derivation)
      showMessage("login-msg", "暗号鍵を導出中...", "info");
      sessionState.encryptionKey = await deriveEncryptionKey(email, password);

      showDashboard();
      loadVault();
    } else {
      const msg = resp.data?.error?.message || "ログインに失敗しました。";
      showMessage("login-msg", msg, "error");
    }
  } catch (err) {
    showMessage("login-msg", "エラー: " + err.message, "error");
  } finally {
    setLoading("login-btn", false);
  }
}

function handleLogout() {
  sessionState = { token: null, userId: null, email: null, encryptionKey: null, vaultVersion: 0 };
  document.getElementById("vault-data").value = "";
  showAuth();
}

// ---------------------------------------------------------------------------
// Vault operations
// ---------------------------------------------------------------------------
async function loadVault() {
  const msgEl = "vault-msg";
  try {
    const resp = await apiGet("/vault");
    if (resp.status !== 200) {
      showMessage(msgEl, "ボールト取得に失敗しました。", "error");
      return;
    }

    sessionState.vaultVersion = resp.data.version;

    if (!resp.data.data || resp.data.data.length === 0 || resp.data.version === 0) {
      // No vault yet
      document.getElementById("vault-data").value = "";
      showMessage(msgEl, "ボールトは空です。データを入力して保存してください。", "info");
      return;
    }

    // Decode base64 data from JSON response
    const blobBytes = base64ToUint8(resp.data.data);
    const plaintext = await decryptVaultData(blobBytes, sessionState.encryptionKey);
    document.getElementById("vault-data").value = JSON.stringify(plaintext, null, 2);
    renderPeople(plaintext);
    showMessage(msgEl, "ボールトを読み込みました (v" + resp.data.version + ")", "success");
  } catch (err) {
    showMessage(msgEl, "復号エラー: " + err.message, "error");
  }
}

// ---------------------------------------------------------------------------
// People (M4)
// ---------------------------------------------------------------------------

// getVaultJSON parses the textarea contents into an object, returning a
// well-formed default if it is empty. This lets callers mutate and
// re-save without clobbering other vault entries.
function getVaultJSON() {
  const raw = document.getElementById("vault-data").value.trim();
  if (!raw) return { version: 1, persons: {} };
  try {
    const parsed = JSON.parse(raw);
    if (!parsed.persons) parsed.persons = {};
    if (!parsed.version) parsed.version = 1;
    return parsed;
  } catch {
    return { version: 1, persons: {} };
  }
}

function setVaultJSON(obj) {
  document.getElementById("vault-data").value = JSON.stringify(obj, null, 2);
}

function renderPeople(vault) {
  const container = document.getElementById("people-list");
  if (!container) return;
  const persons = (vault && vault.persons) || {};
  const ids = Object.keys(persons);
  if (ids.length === 0) {
    container.innerHTML = '<p style="color:var(--muted);font-size:0.85rem;">まだ人物が登録されていません。</p>';
    return;
  }
  const parts = [];
  for (const id of ids) {
    const p = persons[id];
    const name = personDisplayName(p);
    const gda = p.gda_code ? '<div style="font-family:monospace;font-size:0.8rem;color:var(--muted);">' + escapeHTML(p.gda_code) + '</div>' : '';
    parts.push(
      '<div style="border:1px solid var(--border);border-radius:4px;padding:0.6rem;margin-bottom:0.4rem;">' +
      '<div style="font-weight:600;">' + escapeHTML(name) + '</div>' +
      gda +
      '<div style="margin-top:0.4rem;display:flex;gap:0.3rem;flex-wrap:wrap;">' +
      '<button style="flex:0 0 auto;width:auto;padding:0.3rem 0.6rem;font-size:0.8rem;" onclick="handleMintGDA(\'' + id + '\')">GDAコード発行</button>' +
      '<button style="flex:0 0 auto;width:auto;padding:0.3rem 0.6rem;font-size:0.8rem;background:var(--success);" onclick="handleDownloadVCard(\'' + id + '\')">vCardダウンロード</button>' +
      '<button style="flex:0 0 auto;width:auto;padding:0.3rem 0.6rem;font-size:0.8rem;background:var(--danger);" onclick="handleDeletePerson(\'' + id + '\')">削除</button>' +
      '</div></div>'
    );
  }
  container.innerHTML = parts.join("");
}

function personDisplayName(p) {
  if (!p || !p.names || p.names.length === 0) return "(名前なし)";
  // Prefer a latin variant, else first.
  for (const n of p.names) {
    if (n.language_tag && n.language_tag.toLowerCase().includes("latn")) {
      return (n.given || "") + " " + (n.family || "");
    }
  }
  const n = p.names[0];
  return (n.family || "") + " " + (n.given || "");
}

function escapeHTML(s) {
  return String(s || "").replace(/[&<>"']/g, c => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[c]));
}

async function handleCreatePerson(event) {
  event.preventDefault();
  const msgEl = "people-msg";
  clearMessages();

  // Gather form values.
  const nativeFamily = document.getElementById("p-native-family").value.trim();
  const nativeGiven = document.getElementById("p-native-given").value.trim();
  const nativeLang = document.getElementById("p-native-lang").value.trim() || "ja-Jpan";
  const latinFamily = document.getElementById("p-latin-family").value.trim();
  const latinGiven = document.getElementById("p-latin-given").value.trim();
  const country = document.getElementById("p-country").value.trim().toUpperCase();
  const postal = document.getElementById("p-postal").value.trim();
  const region = document.getElementById("p-region").value.trim();
  const locality = document.getElementById("p-locality").value.trim();
  const street = document.getElementById("p-street").value.trim();
  const phone = document.getElementById("p-phone").value.trim();

  if (!nativeFamily && !nativeGiven && !latinFamily && !latinGiven) {
    showMessage(msgEl, "少なくとも氏名をどれか 1 つ入力してください。", "error");
    return;
  }

  setLoading("person-create-btn", true);
  showMessage(msgEl, "サーバーに人物 ID を作成中...", "info");

  try {
    // 1. Ask the server to mint a person row (ID only, no sensitive fields).
    const createResp = await apiPost("/persons", {});
    if (createResp.status !== 201) {
      showMessage(msgEl, "人物作成に失敗しました。", "error");
      return;
    }
    const personID = createResp.data.id;

    // 2. Build the client-side person record and push into the Vault JSON.
    const names = [];
    if (nativeFamily || nativeGiven) {
      names.push({ family: nativeFamily, given: nativeGiven, language_tag: nativeLang });
    }
    if (latinFamily || latinGiven) {
      names.push({ family: latinFamily, given: latinGiven, language_tag: "en-Latn" });
    }
    const addresses = [];
    if (country || postal || region || locality || street) {
      addresses.push({
        script: nativeLang,
        country: country || "",
        address_lines: street ? [street] : [],
        locality: locality,
        administrative_area: region,
        postal_code: postal,
      });
    }
    const phones = phone ? [phone] : [];

    const vault = getVaultJSON();
    vault.persons[personID] = {
      names: names,
      addresses: addresses,
      phones: phones,
      emails: [],
      notes: "",
      gda_code: "",
    };
    setVaultJSON(vault);

    // 3. Re-encrypt and save the Vault.
    showMessage(msgEl, "ボールトを暗号化して保存中...", "info");
    await saveVaultFromJSON(vault);

    // 4. Clear the form and refresh the list.
    event.target.reset();
    document.getElementById("p-native-lang").value = "ja-Jpan";
    document.getElementById("p-country").value = "JP";
    renderPeople(vault);
    showMessage(msgEl, "人物を追加しました。", "success");
  } catch (err) {
    showMessage(msgEl, "エラー: " + err.message, "error");
  } finally {
    setLoading("person-create-btn", false);
  }
}

// saveVaultFromJSON encrypts the given object and performs a PUT /vault
// with the current version. It updates sessionState.vaultVersion on success.
async function saveVaultFromJSON(obj) {
  const blob = await encryptVaultData(obj, sessionState.encryptionKey);
  const blobB64 = uint8ToBase64(blob);
  const resp = await apiPut("/vault", {
    data: blobB64,
    version: sessionState.vaultVersion,
  });
  if (resp.status === 200) {
    sessionState.vaultVersion = resp.data.version;
    return;
  }
  if (resp.status === 409) {
    throw new Error("バージョン競合: 再読込してください");
  }
  throw new Error((resp.data && resp.data.error && resp.data.error.message) || "保存に失敗しました");
}

async function handleDeletePerson(personID) {
  if (!confirm("この人物を削除しますか？")) return;
  const msgEl = "people-msg";
  clearMessages();
  try {
    const resp = await apiDelete("/persons/" + personID);
    if (resp.status !== 204 && resp.status !== 404) {
      showMessage(msgEl, "削除に失敗しました。", "error");
      return;
    }
    const vault = getVaultJSON();
    delete vault.persons[personID];
    setVaultJSON(vault);
    await saveVaultFromJSON(vault);
    renderPeople(vault);
    showMessage(msgEl, "削除しました。", "success");
  } catch (err) {
    showMessage(msgEl, "エラー: " + err.message, "error");
  }
}

async function handleMintGDA(personID) {
  const msgEl = "people-msg";
  clearMessages();
  try {
    const resp = await apiPost("/gda-codes", { person_id: personID });
    if (resp.status !== 201) {
      const m = (resp.data && resp.data.error && resp.data.error.message) || "GDA コード発行に失敗しました。";
      showMessage(msgEl, m, "error");
      return;
    }
    const formatted = resp.data.code;
    const vault = getVaultJSON();
    if (vault.persons[personID]) {
      vault.persons[personID].gda_code = formatted;
      setVaultJSON(vault);
      await saveVaultFromJSON(vault);
      renderPeople(vault);
    }
    showMessage(msgEl, "発行しました: " + formatted, "success");
  } catch (err) {
    showMessage(msgEl, "エラー: " + err.message, "error");
  }
}

async function handleDownloadVCard(personID) {
  const msgEl = "people-msg";
  clearMessages();
  const vault = getVaultJSON();
  const p = vault.persons[personID];
  if (!p) {
    showMessage(msgEl, "人物データが見つかりません。", "error");
    return;
  }

  const payload = {
    names: (p.names || []).map(n => ({
      family: n.family || "",
      given: n.given || "",
      additional: "",
      prefix: "",
      suffix: "",
      language_tag: n.language_tag || "",
    })),
    phones: p.phones || [],
    emails: p.emails || [],
    addresses: (p.addresses || []).map(a => ({
      street_address: (a.address_lines || []).join(", "),
      locality: a.locality || "",
      region: a.administrative_area || "",
      postal_code: a.postal_code || "",
      country: a.country || "",
      language_tag: a.script || "",
    })),
    note: p.notes || "",
    gda_code: p.gda_code || "",
    filename: (personDisplayName(p).trim().replace(/\s+/g, "_") || "person") + ".vcf",
  };

  try {
    const resp = await apiPostRaw("/vcard", payload);
    if (!resp.ok) {
      showMessage(msgEl, "vCard 生成に失敗しました。", "error");
      return;
    }
    const blob = await resp.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = payload.filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    showMessage(msgEl, "ダウンロードしました。", "success");
  } catch (err) {
    showMessage(msgEl, "エラー: " + err.message, "error");
  }
}

async function handleSaveVault() {
  const msgEl = "vault-msg";
  clearMessages();

  const rawText = document.getElementById("vault-data").value.trim();
  if (!rawText) {
    showMessage(msgEl, "データを入力してください。", "error");
    return;
  }

  let parsed;
  try {
    parsed = JSON.parse(rawText);
  } catch {
    showMessage(msgEl, "JSON 形式が不正です。", "error");
    return;
  }

  const saveBtn = document.getElementById("vault-save-btn");
  saveBtn.disabled = true;
  showMessage(msgEl, "暗号化中...", "info");

  try {
    const blob = await encryptVaultData(parsed, sessionState.encryptionKey);
    // Convert to base64 for JSON transport
    const blobB64 = uint8ToBase64(blob);

    showMessage(msgEl, "保存中...", "info");
    const resp = await apiPut("/vault", {
      data: blobB64,
      version: sessionState.vaultVersion,
    });

    if (resp.status === 200) {
      sessionState.vaultVersion = resp.data.version;
      showMessage(msgEl, "保存しました (v" + resp.data.version + ")", "success");
    } else if (resp.status === 409) {
      showMessage(msgEl, "バージョン競合: 他の更新があります。再読込してください。", "error");
    } else {
      const msg = resp.data?.error?.message || "保存に失敗しました。";
      showMessage(msgEl, msg, "error");
    }
  } catch (err) {
    showMessage(msgEl, "暗号化エラー: " + err.message, "error");
  } finally {
    saveBtn.disabled = false;
  }
}

function base64ToUint8(b64) {
  const binary = atob(b64);
  const arr = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) arr[i] = binary.charCodeAt(i);
  return arr;
}

function uint8ToBase64(arr) {
  let binary = "";
  for (let i = 0; i < arr.length; i++) binary += String.fromCharCode(arr[i]);
  return btoa(binary);
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------
// Pre-load argon2 WASM in the background
loadArgon2().catch(err => {
  console.warn("Argon2 pre-load failed (will retry on use):", err.message);
  argon2Ready = false;
});
