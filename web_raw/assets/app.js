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
  const nav = [
    { label: "应用", href: "/" },
    { label: "规则", href: "/rules" }
  ];
  let navHTML = nav
    .map(
      n =>
        `<a href="${n.href}" class="${n.label === current ? "text-white" : "text-sub hover:text-white"} no-underline text-sm font-semibold transition-colors">${n.label}</a>`
    )
    .join("");

  document.getElementById("topbar").innerHTML = `
    <div class="max-w-[1200px] mx-auto px-6 py-3 flex items-center gap-6">
      <a href="/" class="text-lg font-bold text-white no-underline">Serein</a>
      ${navHTML}
      <div class="flex-1"></div>
      <button id="btn-tracker-check" class="px-3 py-1 rounded-lg border border-[rgba(255,255,255,.12)] bg-transparent text-sub text-xs cursor-pointer hover:border-[#FE8A95] hover:text-white transition-colors">当前 Tracker</button>
      <button id="btn-check-all" class="px-3 py-1 rounded-lg border border-[rgba(255,255,255,.12)] bg-transparent text-sub text-xs cursor-pointer hover:border-[#FE8A95] hover:text-white transition-colors">全部更新</button>
      <button id="btn-read-cache" class="px-3 py-1 rounded-lg border border-[rgba(255,255,255,.12)] bg-transparent text-sub text-xs cursor-pointer hover:border-[#FE8A95] hover:text-white transition-colors">读取缓存</button>
    </div>`;
}

// ── 图标 ──
function platformIcon(os) {
  const map = { macos: "apple", windows: "windows", linux: "linux" };
  const name = map[os];
  if (name) return `<img src="/assets/${name}.svg" class="w-4 h-4 inline-block" alt="${os}" title="${os}">`;
  return `<span class="text-xs text-sub font-semibold w-4 h-4 inline-flex items-center justify-center" title="${os}">${os.slice(0, 2).toUpperCase()}</span>`;
}

function iconImg(file, alt, cls) {
  return `<img src="/assets/${file}.svg" class="${cls || "w-4 h-4"} inline-block cursor-pointer hover:opacity-70 transition-opacity" alt="${alt}" title="${alt}">`;
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
