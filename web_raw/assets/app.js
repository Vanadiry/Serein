// Serein shared JS — API 封装、顶栏、公共函数
const API = window.location.origin;

// ── API ──
async function api(path) {
  const r = await fetch(API + path);
  return r.json();
}
async function apiPost(path, body) {
  const r = await fetch(API + path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined
  });
  return r.json();
}

// ── 顶栏 ──
function renderTopbar(current) {
  const tb = document.getElementById("topbar");
  if (!tb) return;
  const path = location.pathname;
  tb.style.cssText = "position:sticky;top:0;z-index:50";

  function navCls(href) {
    if (href === "/" && path === "/") return "bg-[#30303b] text-white";
    if (href !== "/" && path.startsWith(href)) return "bg-[#30303b] text-white";
    return "text-sub hover:bg-[#30303b] hover:text-white";
  }

  const isIndex = path === "/" || path === "/index.html" || path === "";

  tb.innerHTML = `
    <div class="bg-[#1d1d1d] border-b border-[rgba(255,255,255,.06)]">
      <div class="max-w-[1200px] mx-auto px-5 flex items-center gap-3 h-12 relative">
        <h1 class="text-lg font-bold cursor-pointer shrink-0" onclick="location.href='/'">Serein</h1>
        <nav class="flex gap-1 ml-2 items-center">
          <a href="/" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/")}">应用</a>
          <a href="/rules" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/rules")}">规则</a>
        </nav>
        <div class="flex-1"></div>
        ${isIndex ? `
        <nav class="flex gap-1 items-center">
          <button id="btn-tracker-check" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">当前 Tracker</button>
          <button id="btn-check-all" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">全部更新</button>
          <button id="btn-read-cache" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">读取缓存</button>
        </nav>` : ""}
      </div>
    </div>`;
}

// ── 图标 ──
function platformIcon(os, cls) {
  const map = { macos: "apple", windows: "windows", linux: "linux" };
  const name = map[os];
  const sz = cls || "w-5 h-5";
  if (name) return `<span class="icon-bg inline-flex">${iconImgRaw(name, os, sz)}</span>`;
  return `<span class="icon-bg inline-flex"><span class="${sz} text-xs text-sub font-semibold inline-flex items-center justify-center" title="${os}">${os.slice(0, 2).toUpperCase()}</span></span>`;
}

function iconImg(file, alt, cls) {
  return `<span class="icon-bg-pink cursor-pointer hover:opacity-80 transition-opacity">${iconImgRaw(file, alt, cls || "w-5 h-5")}</span>`;
}

function iconYes(alt, cls) {
  return `<span class="icon-bg-green cursor-pointer hover:opacity-80 transition-opacity">${iconImgRaw("yes", alt, cls || "w-5 h-5")}</span>`;
}

function iconImgRaw(file, alt, sz) {
  return `<img src="/assets/${file}.svg" class="${sz} inline-block" alt="${alt}" title="${alt}">`;
}

// ── 工具 ──
function badge(text, bg, fg) {
  return `<span class="${bg} ${fg} px-1.5 py-px rounded text-[10px] font-semibold">${text}</span>`;
}

function formatURL(u) {
  if (!u) return "";
  if (typeof u === "string") return u;
  if (Array.isArray(u)) return u.length > 1 ? `${u[0]} (+${u.length - 1})` : u[0];
  return "";
}

// ── 弹窗 ──
function showModal(html) {
  const el = document.createElement("div");
  el.className =
    "fixed inset-0 z-50 flex items-center justify-center bg-black/60";
  el.onclick = e => { if (e.target === el) el.remove(); };
  el.innerHTML = `<div class="bg-[#2a2a2a] border border-[rgba(255,255,255,.12)] rounded-xl p-6 min-w-[360px] max-w-[480px] shadow-2xl">${html}</div>`;
  document.body.appendChild(el);
}
