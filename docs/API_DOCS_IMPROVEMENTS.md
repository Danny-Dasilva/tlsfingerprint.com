# API Documentation Page Improvements

**File:** `static/docs.html`
**Analysis Date:** 2026-01-07
**Total Improvements:** 50

This document contains a comprehensive list of improvements for the Swagger-style API documentation page, compiled from 5 specialized analysis agents focusing on UI/UX, image handling, Swagger feature parity, performance, and visual design.

---

## Table of Contents

1. [Critical Fixes](#critical-fixes)
2. [High Priority - UI/UX](#high-priority---uiux)
3. [High Priority - Image/Binary Handling](#high-priority---imagebinary-handling)
4. [High Priority - Swagger Feature Parity](#high-priority---swagger-feature-parity)
5. [Medium Priority - Performance & Code Quality](#medium-priority---performance--code-quality)
6. [Medium Priority - Accessibility](#medium-priority---accessibility)
7. [Medium Priority - Mobile Responsiveness](#medium-priority---mobile-responsiveness)
8. [Medium Priority - Visual Design Polish](#medium-priority---visual-design-polish)
9. [Low Priority - Polish](#low-priority---polish)
10. [Implementation Phases](#implementation-phases)

---

## Critical Fixes

### 1. Convert Binary Responses to Blob Instead of Text

**Problem:** Lines 513-522 treat all image responses as text, corrupting binary data.

**Current Code:**
```javascript
if (contentType.includes('image/')) {
  bodyText = `[Binary image data - ${contentType}]`;
  bodyJson = { type: contentType, size: 'binary' };
}
```

**Fixed Code:**
```javascript
if (contentType.includes('image/') || contentType.includes('application/octet-stream') || contentType.includes('application/pdf')) {
  const blob = await response.blob();
  isBinary = true;
  bodyJson = { type: contentType, size: blob.size, dataUrl: URL.createObjectURL(blob) };
  bodyText = blob.size;
}
```

**Why:** Using `response.blob()` preserves binary integrity for proper image display.

---

### 2. Display Actual Images Instead of Placeholder Text

**Problem:** Line 524 displays `[Binary image data...]` instead of rendering images.

**Current Code:**
```javascript
document.getElementById(`${id}-response-body`).innerHTML = bodyText;
```

**Fixed Code:**
```javascript
let displayContent = bodyText;
if (contentType.includes('image/') && bodyJson?.dataUrl) {
  displayContent = `<div class="bg-gray-100 p-4 rounded flex items-center justify-center max-h-96">
    <img src="${bodyJson.dataUrl}" alt="Response image" class="max-w-full max-h-full object-contain" />
  </div>
  <div class="text-xs text-gray-500 mt-2">Size: ${(bodyJson.size / 1024).toFixed(2)} KB | Type: ${contentType}</div>`;
}
document.getElementById(`${id}-response-body`).innerHTML = displayContent;
```

**Why:** Users can see actual PNG, JPEG, GIF images directly in the response panel.

---

### 3. Add Request Body Editor for POST/PUT/PATCH

**Problem:** No UI exists for entering request body payloads.

**Add after line 220:**
```javascript
${hasRequestBody ? `
  <div class="mb-4 border-b border-swagger-border pb-4" id="${id}-request-body-section">
    <h4 class="text-sm font-semibold text-swagger-text mb-2">Request body</h4>
    <textarea
      id="${id}-request-body"
      class="w-full border border-swagger-border rounded px-3 py-2 text-sm font-mono hidden"
      rows="10"
      placeholder='${requestBody?.content?.['application/json']?.schema?.example ?
        JSON.stringify(requestBody.content['application/json'].schema.example, null, 2) :
        '{}'}'
    ></textarea>
    <div class="text-xs text-gray-500 param-desc" id="${id}-request-body-desc">
      ${requestBody?.description || 'Request payload'}
    </div>
  </div>
` : ''}
```

**Why:** POST/PUT requests are non-functional without body editing capability.

---

### 4. Add Authentication UI (Bearer Token, API Key)

**Problem:** No way to set Authorization headers for protected endpoints.

**Add to header section (after line 119):**
```html
<button onclick="openAuthModal()" class="px-4 py-2 text-sm border border-swagger-border rounded hover:bg-gray-50 font-semibold text-swagger-execute">
  Authorize
</button>

<!-- Auth Modal -->
<div id="auth-modal" class="fixed inset-0 bg-black bg-opacity-50 hidden z-50" onclick="closeAuthModal(event)">
  <div class="absolute right-0 top-0 h-full w-96 bg-white shadow-lg p-6 overflow-y-auto" onclick="event.stopPropagation()">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-lg font-bold text-swagger-text">Available authorizations</h2>
      <button onclick="closeAuthModal()" class="text-2xl text-gray-400">&times;</button>
    </div>

    <div class="space-y-4">
      <div class="border border-swagger-border rounded p-4">
        <h3 class="text-sm font-semibold text-swagger-text mb-2">Bearer Token</h3>
        <input type="password" id="auth-bearer" placeholder="Enter bearer token"
          class="w-full border border-swagger-border rounded px-3 py-2 text-sm font-mono mb-2">
        <button onclick="setAuthBearer()" class="w-full bg-swagger-execute text-white text-sm font-semibold py-2 rounded hover:opacity-90">
          Apply
        </button>
      </div>

      <div class="border border-swagger-border rounded p-4">
        <h3 class="text-sm font-semibold text-swagger-text mb-2">API Key</h3>
        <input type="text" id="auth-apikey" placeholder="Enter API key"
          class="w-full border border-swagger-border rounded px-3 py-2 text-sm font-mono mb-2">
        <select id="auth-apikey-in" class="w-full border border-swagger-border rounded px-3 py-2 text-sm mb-2">
          <option value="header">Header (X-API-Key)</option>
          <option value="query">Query Parameter</option>
        </select>
        <button onclick="setAuthApiKey()" class="w-full bg-swagger-execute text-white text-sm font-semibold py-2 rounded hover:opacity-90">
          Apply
        </button>
      </div>
    </div>
  </div>
</div>
```

**Add JavaScript:**
```javascript
const authState = {
  bearerToken: null,
  apiKey: null,
  apiKeyIn: 'header',
};

function openAuthModal() {
  document.getElementById('auth-modal').classList.remove('hidden');
}

function closeAuthModal(event) {
  if (!event || event.target.id === 'auth-modal') {
    document.getElementById('auth-modal').classList.add('hidden');
  }
}

function setAuthBearer() {
  authState.bearerToken = document.getElementById('auth-bearer').value;
  if (authState.bearerToken) closeAuthModal();
}

function setAuthApiKey() {
  authState.apiKey = document.getElementById('auth-apikey').value;
  authState.apiKeyIn = document.getElementById('auth-apikey-in').value;
  if (authState.apiKey) closeAuthModal();
}

// In executeRequest, use auth:
if (authState.bearerToken) {
  headers['Authorization'] = `Bearer ${authState.bearerToken}`;
}
if (authState.apiKey && authState.apiKeyIn === 'header') {
  headers['X-API-Key'] = authState.apiKey;
}
```

---

### 5. Add Custom Headers Input Panel

**Problem:** Headers are hardcoded; users can't add custom headers.

**Add after parameters section (line 253):**
```html
<div class="mb-4 border-b border-swagger-border pb-4">
  <div class="flex items-center justify-between mb-2">
    <h4 class="text-sm font-semibold text-swagger-text">Headers</h4>
    <button onclick="addCustomHeader('${id}')" class="px-2 py-1 text-xs bg-blue-50 border border-swagger-get text-swagger-get rounded hover:bg-blue-100">
      Add Header
    </button>
  </div>
  <div id="${id}-headers-container">
    <div class="text-xs text-gray-500">No custom headers added</div>
  </div>
</div>
```

**Add JavaScript:**
```javascript
function addCustomHeader(id) {
  const container = document.getElementById(`${id}-headers-container`);
  if (!container.querySelector('.header-input-row')) {
    container.innerHTML = '';
  }
  const headerId = `${id}-header-${Date.now()}`;
  const html = `
    <div class="header-input-row flex gap-2 mb-2" id="${headerId}">
      <input type="text" placeholder="Header name" class="flex-1 border border-swagger-border rounded px-2 py-1 text-sm" id="${headerId}-name">
      <input type="text" placeholder="Value" class="flex-2 border border-swagger-border rounded px-2 py-1 text-sm" id="${headerId}-value">
      <button onclick="removeHeader('${headerId}')" class="px-2 py-1 text-xs bg-red-50 text-red-600 rounded hover:bg-red-100">x</button>
    </div>
  `;
  container.insertAdjacentHTML('beforeend', html);
}
```

---

## High Priority - UI/UX

### 6. Add Loading Spinner During Request Execution

**Location:** Lines 491-493

**Current:**
```javascript
document.getElementById(`${id}-status-code`).textContent = '...';
document.getElementById(`${id}-response-body`).innerHTML = 'Loading...';
```

**Fixed:**
```javascript
const statusEl = document.getElementById(`${id}-status-code`);
const bodyEl = document.getElementById(`${id}-response-body`);
statusEl.innerHTML = `<span class="flex items-center gap-2">
  <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
  </svg>
  Sending...
</span>`;
bodyEl.innerHTML = `<div class="flex items-center gap-2 text-gray-600">
  <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
  </svg>
  Waiting for response...
</div>`;
```

---

### 7. Add Comprehensive Error Display with Types

**Location:** Lines 530-533

**Fixed:**
```javascript
catch (error) {
  const statusEl = document.getElementById(`${id}-status-code`);
  const bodyEl = document.getElementById(`${id}-response-body`);

  let errorType = 'Request Failed';
  let errorDetail = error.message;

  if (error instanceof TypeError && error.message.includes('fetch')) {
    errorType = 'Network Error';
    errorDetail = 'Unable to connect. Check your internet connection.';
  } else if (error.name === 'AbortError') {
    errorType = 'Request Timeout';
    errorDetail = 'The request took too long. Try again.';
  }

  statusEl.innerHTML = `<span class="flex items-center gap-2 text-red-600">
    <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
      <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd"/>
    </svg>
    ${errorType}
  </span>`;

  bodyEl.innerHTML = `<div class="bg-red-50 border-l-4 border-red-500 p-4 rounded">
    <p class="text-red-700 font-semibold text-sm">${errorType}</p>
    <p class="text-red-600 text-sm mt-1">${errorDetail}</p>
  </div>`;
}
```

---

### 8. Add Execute Button Loading State

**Location:** Lines 259-262

**Add at start of executeRequest:**
```javascript
async function executeRequest(path, method, id) {
  const executeBtn = document.querySelector(`button[onclick="executeRequest('${path}', '${method}', '${id}')"]`);
  const clearBtn = executeBtn.nextElementSibling;

  executeBtn.disabled = true;
  clearBtn.disabled = true;
  executeBtn.innerHTML = `
    <svg class="w-4 h-4 animate-spin inline mr-2" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
    </svg>
    Executing...
  `;
  executeBtn.classList.add('opacity-50');

  // ... rest of function ...

  // At end (success or error):
  executeBtn.innerHTML = 'Execute';
  executeBtn.disabled = false;
  clearBtn.disabled = false;
  executeBtn.classList.remove('opacity-50');
}
```

---

### 9. Add Success Toast Notification

**Add after line 528:**
```javascript
const toast = document.createElement('div');
toast.className = 'fixed bottom-4 right-4 bg-green-500 text-white px-4 py-3 rounded shadow-lg flex items-center gap-2';
toast.innerHTML = `
  <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/>
  </svg>
  <span>Request successful - ${response.status}</span>
`;
document.body.appendChild(toast);
setTimeout(() => toast.remove(), 3000);
```

---

### 10. Add Copy to Clipboard Buttons

**Replace curl section (lines 273-275):**
```html
<div class="mb-4">
  <div class="flex items-center justify-between mb-2">
    <h5 class="text-sm font-semibold text-swagger-text">Curl</h5>
    <button onclick="copyToClipboard(document.getElementById('${id}-curl').textContent)" class="px-3 py-1 text-xs border border-swagger-border rounded hover:bg-gray-50">
      Copy
    </button>
  </div>
  <pre class="code-block rounded p-4 text-sm font-mono overflow-x-auto whitespace-pre-wrap" id="${id}-curl"></pre>
</div>
```

**Add JavaScript:**
```javascript
async function copyToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text);
    const toast = document.createElement('div');
    toast.className = 'fixed bottom-4 right-4 bg-blue-500 text-white px-4 py-2 rounded shadow-lg text-sm';
    toast.textContent = 'Copied to clipboard!';
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), 2000);
  } catch (err) {
    console.error('Failed to copy:', err);
  }
}
```

---

## High Priority - Image/Binary Handling

### 11. Content-Type Detection for All Binary Formats

**Add helper function:**
```javascript
function isBinaryContentType(contentType) {
  const binaryTypes = [
    'image/',
    'audio/',
    'video/',
    'application/pdf',
    'application/octet-stream',
    'application/x-protobuf',
    'application/x-gzip',
    'application/zip',
    'font/'
  ];
  return binaryTypes.some(type => contentType.includes(type));
}
```

---

### 12. Smart File Extension Mapping for Downloads

**Replace lines 160-171:**
```javascript
function downloadResponse(id) {
  const data = state.responses[id];
  if (!data) return;

  const mimeToExt = {
    'image/png': 'png',
    'image/jpeg': 'jpg',
    'image/gif': 'gif',
    'image/webp': 'webp',
    'application/pdf': 'pdf',
    'application/zip': 'zip',
    'audio/mpeg': 'mp3',
    'video/mp4': 'mp4'
  };

  const contentType = data.body?.type || '';
  const ext = Object.entries(mimeToExt).find(([mime]) => contentType.includes(mime))?.[1] || 'json';
  const filename = `response.${ext}`;

  let blobData, blobType;
  if (data.body?.dataUrl) {
    fetch(data.body.dataUrl).then(r => r.blob()).then(blob => {
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      a.click();
      URL.revokeObjectURL(url);
    });
  } else {
    blobData = JSON.stringify(data.body, null, 2);
    const blob = new Blob([blobData], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }
}
```

---

### 13. Store Binary Blob Data for Re-Download

**Modify line 527-528:**
```javascript
state.responses[id] = {
  body: bodyJson,
  headers: responseHeaders,
  isBinary: isBinary,
  contentType: contentType,
  originalBlob: isBinary ? blob : null
};
```

---

### 14. Add Response Body Size Display

**Modify response body section (lines 300-307):**
```html
<div class="flex items-center justify-between mb-2">
  <div class="flex items-center gap-2">
    <span class="text-sm font-semibold text-swagger-text">Response body</span>
    <span class="text-xs text-gray-500" id="${id}-body-size"></span>
  </div>
  <button onclick="downloadResponse('${id}')" class="px-3 py-1 text-xs border border-swagger-border rounded hover:bg-gray-50">
    Download
  </button>
</div>
```

**Add size calculation after response:**
```javascript
const sizeStr = bodyJson.size > 1024 * 1024
  ? `${(bodyJson.size / 1024 / 1024).toFixed(2)} MB`
  : `${(bodyJson.size / 1024).toFixed(2)} KB`;
document.getElementById(`${id}-body-size`).textContent = `(${sizeStr})`;
```

---

### 15. Add Image Metadata Display (Dimensions)

**Enhanced image display:**
```javascript
if (contentType.includes('image/') && bodyJson?.dataUrl) {
  displayContent = `<div class="bg-gray-100 p-4 rounded">
    <div class="mb-4 flex items-center justify-center max-h-96">
      <img id="${id}-image-preview" src="${bodyJson.dataUrl}" alt="Response image"
        class="max-w-full max-h-full object-contain" onload="updateImageMetadata('${id}')" />
    </div>
    <div class="text-xs text-gray-500 space-y-1">
      <div>Size: ${(bodyJson.size / 1024).toFixed(2)} KB</div>
      <div>Type: ${contentType}</div>
      <div id="${id}-image-dims" class="hidden">Dimensions: <span id="${id}-image-dims-text"></span></div>
    </div>
  </div>`;
}

function updateImageMetadata(id) {
  const img = document.getElementById(`${id}-image-preview`);
  if (img && img.naturalWidth) {
    document.getElementById(`${id}-image-dims`).classList.remove('hidden');
    document.getElementById(`${id}-image-dims-text`).textContent = `${img.naturalWidth}x${img.naturalHeight}px`;
  }
}
```

---

## High Priority - Swagger Feature Parity

### 16. Add File Upload Input Type

**Replace lines 240-246:**
```javascript
let inputHtml;
if (p.schema?.format === 'binary' || p.schema?.type === 'file') {
  inputHtml = `<input type="file" id="${id}-param-${p.name}" class="param-input w-full border border-swagger-border rounded px-3 py-2 text-sm hidden">`;
} else if (p.schema?.enum) {
  inputHtml = `<select id="${id}-param-${p.name}" class="param-input w-full border border-swagger-border rounded px-3 py-2 text-sm hidden">
    ${p.schema.enum.map(e => `<option value="${e}">${e}</option>`).join('')}
  </select>`;
} else {
  inputHtml = `<input type="text" id="${id}-param-${p.name}" class="param-input w-full border border-swagger-border rounded px-3 py-2 text-sm font-mono hidden" placeholder="${p.schema?.type || 'value'}">`;
}
```

---

### 17. Add Schema Display Panel with Tabs

**Replace responses section (lines 322-348):**
```html
<div class="border-t border-swagger-border pt-4">
  <div class="border border-swagger-border rounded">
    <div class="flex border-b border-swagger-border bg-gray-50">
      <button onclick="switchResponseTab('${id}', 'example')" class="px-4 py-2 text-sm border-b-2 border-swagger-execute font-semibold text-swagger-execute" data-tab="example">
        Example Value
      </button>
      <button onclick="switchResponseTab('${id}', 'schema')" class="px-4 py-2 text-sm border-b-2 border-transparent text-gray-500 hover:text-gray-700" data-tab="schema">
        Schema
      </button>
    </div>

    <div id="${id}-example-tab" class="p-4">
      <pre class="code-block rounded p-3 text-xs font-mono overflow-x-auto">
        ${details.responses?.[200]?.content?.['application/json']?.example ?
          JSON.stringify(details.responses[200].content['application/json'].example, null, 2) :
          '{...}'}
      </pre>
    </div>

    <div id="${id}-schema-tab" class="p-4 hidden">
      <pre class="code-block rounded p-3 text-xs font-mono">${renderSchema(details.responses?.[200]?.content?.['application/json']?.schema)}</pre>
    </div>
  </div>
</div>
```

---

### 18. Add Example Values in Parameter Descriptions

**Replace line 247:**
```html
<div class="text-gray-500 text-sm">
  ${p.description || '-'}
  ${p.schema?.example ? `<br/><span class="text-xs text-blue-600 font-mono">Example: <code class="bg-blue-50 px-1">${p.schema.example}</code></span>` : ''}
  ${p.schema?.default ? `<br/><span class="text-xs text-gray-500 font-mono">Default: <code>${p.schema.default}</code></span>` : ''}
</div>
```

---

### 19. Add Request/Response Timing Metrics

**Modify executeRequest:**
```javascript
async function executeRequest(path, method, id) {
  const startTime = performance.now();

  // ... existing code ...

  const endTime = performance.now();
  const duration = Math.round(endTime - startTime);

  document.getElementById(`${id}-status-code`).innerHTML = `
    <div class="flex items-center gap-2">
      <span class="font-bold">${response.status}</span>
      <span class="text-xs text-gray-500">${duration}ms</span>
    </div>
  `;
}
```

---

### 20. Professional Syntax Highlighting (highlight.js)

**Add to head:**
```html
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11/dist/highlight.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11/dist/languages/json.min.js"></script>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/highlight.js@11/styles/atom-one-dark.min.css">
```

**Replace formatJson:**
```javascript
function formatJson(obj) {
  const json = JSON.stringify(obj, null, 2);
  return hljs.highlight(json, { language: 'json', ignoreIllegals: true }).value;
}
```

---

## Medium Priority - Performance & Code Quality

### 21. Replace innerHTML += with DocumentFragment

**Replace lines 558-574:**
```javascript
const container = document.getElementById('api-sections');
container.innerHTML = '';
const sectionsHtml = [];

tagOrder.forEach(tag => {
  if (grouped[tag]) {
    sectionsHtml.push(buildSection(tag, grouped[tag], tagIndex++));
    renderedTags.add(tag);
  }
});

Object.keys(grouped).forEach(tag => {
  if (!renderedTags.has(tag)) {
    sectionsHtml.push(buildSection(tag, grouped[tag], tagIndex++));
  }
});

container.innerHTML = sectionsHtml.join('');
```

---

### 22. Fix XSS Vulnerability in formatJson

**Fixed formatJson:**
```javascript
function formatJson(obj) {
  const json = JSON.stringify(obj, null, 2);

  // First escape HTML
  let escaped = json
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');

  // Then add color spans (safe because already escaped)
  escaped = escaped
    .replace(/&quot;([^&]*)&quot;:/g, '<span class="text-cyan-300">&quot;$1&quot;</span>:')
    .replace(/: &quot;([^&]*)&quot;/g, ': <span class="text-green-300">&quot;$1&quot;</span>')
    .replace(/: (\d+)(?=[,\n]|$)/g, ': <span class="text-pink-300">$1</span>')
    .replace(/: (true|false|null)(?=[,\n]|$)/g, ': <span class="text-yellow-300">$1</span>');

  return escaped;
}
```

---

### 23. Cache DOM Elements in executeRequest

**At start of executeRequest:**
```javascript
async function executeRequest(path, method, id) {
  const elements = {
    responseDisplay: document.getElementById(`${id}-response-display`),
    curl: document.getElementById(`${id}-curl`),
    requestUrl: document.getElementById(`${id}-request-url`),
    statusCode: document.getElementById(`${id}-status-code`),
    responseBody: document.getElementById(`${id}-response-body`),
    responseHeaders: document.getElementById(`${id}-response-headers`),
    params: Array.from(document.querySelectorAll(`[id^="${id}-param-"]`))
  };

  // Use elements.curl, elements.statusCode, etc. throughout
}
```

---

### 24. Event Delegation Instead of Inline Handlers

**Add after loadSpec renders:**
```javascript
document.getElementById('api-sections').addEventListener('click', (e) => {
  if (e.target.closest('[data-id]') && !e.target.closest('button') && !e.target.closest('input')) {
    const row = e.target.closest('[data-id]');
    if (row) toggleEndpoint(row.dataset.id);
  }

  if (e.target.closest('[data-section] > button')) {
    const sectionId = e.target.closest('[data-section]').dataset.section;
    toggleSection(sectionId);
  }

  if (e.target.matches('[id$="-try-btn"]')) {
    const id = e.target.id.replace('-try-btn', '');
    toggleTryItOut(id);
  }
});
```

---

### 25. Wrap Functions in Module Pattern (IIFE)

**Wrap entire script:**
```javascript
(function() {
  'use strict';

  const methodConfig = { ... };
  const state = { ... };

  // All functions here

  // Export only what's needed for inline handlers
  window.toggleSection = toggleSection;
  window.toggleEndpoint = toggleEndpoint;
  window.toggleTryItOut = toggleTryItOut;
  window.executeRequest = executeRequest;
  window.downloadResponse = downloadResponse;

  document.addEventListener('DOMContentLoaded', loadSpec);
})();
```

---

### 26. Add Try-Finally for Blob URL Cleanup

**Fixed downloadResponse:**
```javascript
function downloadResponse(id) {
  const data = state.responses[id];
  if (!data) return;

  let url;
  try {
    const blob = new Blob([JSON.stringify(data.body, null, 2)], { type: 'application/json' });
    url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `response-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  } catch (error) {
    console.error('Download failed:', error);
  } finally {
    if (url) URL.revokeObjectURL(url);
  }
}
```

---

## Medium Priority - Accessibility

### 27. Add Aria-Labels to Collapsible Sections

**Update section button (lines 373-382):**
```html
<button onclick="toggleSection('${sectionId}')"
  class="w-full flex items-center justify-between px-4 py-3 hover:bg-gray-50 text-left focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-blue-500 rounded"
  aria-expanded="false"
  id="${sectionId}-btn"
  aria-label="Toggle ${tag} section">
```

**Update toggleSection:**
```javascript
function toggleSection(id) {
  const content = document.getElementById(`${id}-content`);
  const chevron = document.getElementById(`${id}-chevron`);
  const btn = document.getElementById(`${id}-btn`);

  const isOpen = !content.classList.contains('hidden');
  content.classList.toggle('hidden');
  chevron.classList.toggle('rotate-90');
  btn.setAttribute('aria-expanded', !isOpen);
}
```

---

### 28. Add Keyboard Navigation Focus States

**Add to CSS:**
```css
button:focus-visible {
  outline: 2px solid #4990e2;
  outline-offset: 2px;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}
```

---

### 29. Add Accessible Labels to Parameter Inputs

**Update parameter inputs:**
```html
<label for="${id}-param-${p.name}" class="sr-only">
  ${p.name} (${p.schema?.type || 'string'}) - ${p.description || 'Parameter'}
</label>
<input type="text"
  id="${id}-param-${p.name}"
  class="param-input w-full border border-swagger-border rounded px-3 py-2 text-sm font-mono hidden"
  aria-label="${p.name}: ${p.description || 'Parameter'}"
  ${p.required ? 'required aria-required="true"' : ''}>
```

---

### 30. Add Visual Indicators for Required Parameters

**Update parameter rows:**
```javascript
${params.filter(p => p.required).map(p => `
  <tr class="bg-red-50 border-l-2 border-red-500">
    <td class="align-top">
      <div class="font-semibold text-swagger-text flex items-center gap-2">
        ${p.name}
        <span class="px-2 py-0.5 text-xs bg-red-600 text-white rounded">Required</span>
      </div>
      <div class="text-xs text-gray-500">${p.schema?.type || 'string'}</div>
      <div class="text-xs text-gray-400">(${p.in})</div>
    </td>
    ...
  </tr>
`).join('')}
```

---

## Medium Priority - Mobile Responsiveness

### 31. Mobile Responsive Tables

**Wrap tables:**
```html
<div class="overflow-x-auto">
  <table class="w-full border-collapse text-sm min-w-min">
    ...
  </table>
</div>
```

---

### 32. Mobile Responsive Code Blocks

**Update code blocks:**
```html
<pre class="code-block rounded p-4 text-sm font-mono overflow-x-auto max-h-60 sm:max-h-96 break-words whitespace-pre-wrap"></pre>
```

**Add CSS:**
```css
@media (max-width: 640px) {
  .endpoint-row { margin-bottom: 0.5rem; }
  .param-table { font-size: 11px; }
  .code-block { font-size: 11px; padding: 8px 12px; }
}
```

---

### 33. Mobile Header Stacking

**Update header (lines 101-102):**
```html
<div class="flex flex-col sm:flex-row items-start sm:items-center gap-2 mb-2">
  <h1 class="text-3xl sm:text-4xl font-bold text-swagger-text">tlsfingerprint.com</h1>
  <span class="px-2 py-0.5 text-xs bg-gray-200 text-gray-700 rounded">1.0.0</span>
</div>
```

---

## Medium Priority - Visual Design Polish

### 34. Add Smooth Button Transitions

**Add to CSS:**
```css
button {
  transition: all 0.2s ease-in-out;
}
```

---

### 35. Dark Mode Support via CSS Variables

**Add to CSS:**
```css
:root {
  --color-bg: #fafafa;
  --color-text: #3b4151;
  --color-border: #d8dde7;
  --color-code-bg: #41444e;
}

@media (prefers-color-scheme: dark) {
  :root {
    --color-bg: #1e1e1e;
    --color-text: #e0e0e0;
    --color-border: #404040;
    --color-code-bg: #0d1117;
  }
}

body { background: var(--color-bg); color: var(--color-text); }
.code-block { background: var(--color-code-bg); }
```

---

### 36. Input Field Focus State Styling

**Add to CSS:**
```css
.param-input:focus {
  outline: none;
  border-color: #61affe !important;
  box-shadow: 0 0 0 3px rgba(97, 175, 254, 0.1);
}
```

---

### 37. Consistent Button Spacing with Gap

**Update execute/clear buttons (lines 258-267):**
```html
<div class="flex gap-2">
  <button class="flex-1 bg-swagger-execute text-white font-bold py-3 px-4 text-sm rounded-l hover:bg-opacity-90 transition-colors">
    Execute
  </button>
  <button class="flex-1 bg-gray-400 text-white font-bold py-3 px-4 text-sm rounded-r hover:bg-opacity-90 transition-colors">
    Clear
  </button>
</div>
```

---

### 38. Method Badge Visual Enhancement

**Update method badges (lines 198-200):**
```html
<span class="${config.badge} text-white text-xs font-extrabold uppercase px-3 py-1.5 rounded-md min-w-[70px] text-center shadow-md">
  ${method}
</span>
```

**Add CSS:**
```css
.endpoint-row [class*="bg-swagger"] {
  font-family: 'Source Code Pro', monospace;
  letter-spacing: 0.5px;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}
```

---

## Low Priority - Polish

### 39. Print-Friendly Stylesheet

```css
@media print {
  header { border-bottom: 2px solid #000; }
  .endpoint-row svg, button, select { display: none !important; }
  .endpoint-content { max-height: none !important; display: block !important; }
  pre { white-space: pre-wrap; page-break-inside: avoid; }
}
```

---

### 40. Loading State Visual Hierarchy

**Update loading indicator (lines 124-130):**
```html
<div class="flex items-center justify-center py-12">
  <svg class="w-6 h-6 animate-spin text-swagger-execute mr-3" fill="none" viewBox="0 0 24 24">
    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
  </svg>
  <span class="text-sm font-semibold text-swagger-text">Loading API specification...</span>
</div>
```

---

### 41-50. Additional Polish Items

| # | Category | Improvement |
|---|----------|-------------|
| 41 | Visual | Consistent gap-based layout system |
| 42 | Visual | Color contrast & typography hierarchy |
| 43 | Visual | SVG icon transition timing (0.3s cubic-bezier) |
| 44 | Code | Eliminate repeated DOM queries in toggle |
| 45 | Code | Optimize max-height animation with calculated height |
| 46 | Code | Reorganize state management (Set/Map consistency) |
| 47 | Code | Add error boundary for JSON parsing with retry |
| 48 | Image | Validate binary signature (magic bytes) |
| 49 | Image | Handle CORS for binary responses |
| 50 | Image | Preview fallback for unsupported formats |

---

## Implementation Phases

### Phase 1: Critical Fixes
- Image/binary response handling (#1-3, #11-15)
- POST body editor (#3)
- Authentication UI (#4-5)

### Phase 2: UX Polish
- Loading states and spinners (#6, #8)
- Error handling (#7)
- Copy buttons (#10)
- Timing metrics (#19)

### Phase 3: Accessibility & Mobile
- Aria labels and keyboard nav (#27-30)
- Mobile responsiveness (#31-33)

### Phase 4: Performance
- DOM optimization (#21, #23-26)

### Phase 5: Visual Polish
- Dark mode, transitions, spacing (#34-38)

---

## References

- httpbin.org Swagger UI
- Swagger UI official documentation
- WCAG 2.1 accessibility guidelines
- Tailwind CSS documentation
