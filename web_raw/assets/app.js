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
          <button id="btn-read-cache" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">读取缓存</button>
          <button id="btn-check-all" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">检查全部</button>
          <button id="btn-tracker-check" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">检查当前 Tracker</button>
        </nav>` : `
        <nav class="flex gap-1 items-center">
          <button id="btn-search-mode" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">搜索规则</button>
          <button id="btn-sync" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">同步规则</button>
        </nav>`}
      </div>
    </div>`;
}

// ── 图标 ──
function platformLabel(os) {
  const labels = { macos: "macOS", windows: "Windows", linux: "Linux", android: "Android", ios: "iOS" };
  return labels[os] || os;
}

function platformIcon(os, cls) {
  const map = { macos: "apple", windows: "windows", linux: "linux", ios: "ios", android: "android" };
  const name = map[os];
  const label = platformLabel(os);
  const sz = cls || "w-5 h-5";
  if (name) return `<span class="icon-bg inline-flex">${iconImgRaw(name, label, sz)}</span>`;
  return `<span class="icon-bg inline-flex"><span class="${sz} text-xs text-sub font-semibold inline-flex items-center justify-center" title="${label}">${os.slice(0, 2).toUpperCase()}</span></span>`;
}

function iconImg(file, alt, cls) {
  return `<span class="icon-bg-pink cursor-pointer hover:opacity-80 transition-opacity">${iconImgRaw(file, alt, cls || "w-5 h-5")}</span>`;
}

function iconYes(alt, cls) {
  return `<span class="icon-bg-green cursor-pointer hover:opacity-80 transition-opacity">${iconImgRaw("yes", alt, cls || "w-5 h-5")}</span>`;
}

function iconImgRaw(file, alt, sz) {
  return `<img src="/assets/${file}.svg" class="${sz} inline-block" alt="${alt}" title="${alt}" draggable="false">`;
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

// ── 通知组件 ──
var _currentToast = null;

function _makeToast(title, body, titleBg, bodyBg, autoCloseSec) {
  if (_currentToast) {
    var old = _currentToast;
    old.style.opacity = "0";
    setTimeout(function() { old.remove(); }, 300);
  }

  var el = document.createElement("div");
  el.className = "fixed bottom-4 right-4 z-[100] rounded-lg shadow-xl text-sm transition-opacity duration-200";
  el.style.opacity = "0";
  el.style.maxWidth = "calc(100vw - 32px)";
  el.innerHTML =
    '<div class="' + titleBg + ' text-white font-semibold px-4 py-2 rounded-t-lg flex items-center justify-between">' +
    '<span>' + title + '</span>' +
    '<span class="cursor-pointer text-white opacity-60 hover:opacity-100 text-base leading-none ml-3">✕</span>' +
    '</div>' +
    '<div class="' + bodyBg + ' text-white px-4 py-2 rounded-b-lg">' + (body || "") + '</div>';
  document.body.appendChild(el);
  requestAnimationFrame(function() { el.style.opacity = "1"; });

  var timer = null;
  function close() {
    if (timer) clearTimeout(timer);
    el.style.opacity = "0";
    setTimeout(function() { el.remove(); }, 300);
    if (_currentToast === el) _currentToast = null;
    closed = true;
  }
  var closed = false;

  el.querySelector("span[class*='cursor-pointer']").onclick = close;

  if (autoCloseSec > 0) {
    timer = setTimeout(close, autoCloseSec * 1000);
  }

  _currentToast = el;

  return {
    el: el,
    done: function(okBody, isError, titleText) {
      if (closed) {
        var t = _makeToast(isError ? "错误" : "成功", okBody, isError ? "bg-[#dc2626]" : "bg-[#68a868]", isError ? "bg-[#dc2626]/80" : "bg-[#68a868]/80", isError ? 0 : 5);
        if (titleText) t.el.querySelector("span:first-child").textContent = titleText;
        return;
      }
      var tb = isError ? "bg-[#dc2626]" : "bg-[#68a868]";
      var bb = isError ? "bg-[#dc2626]/80" : "bg-[#68a868]/80";
      var tt = titleText || (isError ? "错误" : "成功");
      el.querySelector("div:first-child").className = tb + " text-white font-semibold px-4 py-2 rounded-t-lg flex items-center justify-between";
      el.querySelector("span:first-child").textContent = tt;
      el.querySelector("div:last-child").className = bb + " text-white px-4 py-2 rounded-b-lg";
      el.querySelector("div:last-child").innerHTML = okBody || "";
      if (isError) {
        if (timer) clearTimeout(timer);
      } else {
        if (timer) clearTimeout(timer);
        timer = setTimeout(close, 5000);
      }
    }
  };
}

function showLoading(title, body) {
  return _makeToast(title || "加载中", body || "", "bg-[#e8a040]", "bg-[#e8a040]/80", 0);
}

// ── 通用悬停提示（全局单例）──
var _tooltipEl = null;
function _ensureTooltip() {
  if (!_tooltipEl) {
    _tooltipEl = document.createElement("div");
    _tooltipEl.className = "fixed bg-[#1c1c22] border border-[rgba(255,255,255,.15)] text-xs text-sub rounded-lg px-3 py-2 shadow-xl pointer-events-none z-[200] transition-opacity duration-150";
    _tooltipEl.style.display = "none";
    _tooltipEl.style.opacity = "0";
    _tooltipEl.style.maxWidth = (window.innerWidth - 32) + "px";
    _tooltipEl.style.whiteSpace = "normal";
    _tooltipEl.style.wordBreak = "break-all";
    _tooltipEl.style.lineHeight = "1.4";
    document.body.appendChild(_tooltipEl);
  }
  return _tooltipEl;
}

function _positionTooltip(e) {
  var tip = _ensureTooltip();
  var rect = e.target.getBoundingClientRect();
  var left = rect.left + rect.width / 2;
  tip.style.left = left + "px";
  tip.style.top = (rect.top - 8) + "px";
  tip.style.transform = "translate(-50%, -100%)";
  tip.style.display = "block";
  requestAnimationFrame(function() { tip.style.opacity = "1"; });

  var tipRect = tip.getBoundingClientRect();
  if (tipRect.right > window.innerWidth - 16) {
    tip.style.left = "auto";
    tip.style.right = "16px";
    tip.style.transform = "translate(0, -100%)";
  }
  if (tipRect.left < 16) {
    tip.style.left = "16px";
    tip.style.transform = "translate(0, -100%)";
  }
}

function showTooltip(e, html) {
  // 若 html 未传，从 data-tip 读取
  if (!html && e && e.target) {
    var el = e.target.closest("[data-tip]");
    if (el) html = el.getAttribute("data-tip");
  }
  if (!html) return;
  _ensureTooltip().innerHTML = html;
  _positionTooltip(e);
}

function hideTooltip() {
  if (_tooltipEl) { _tooltipEl.style.opacity = "0"; }
}

// 链接悬停（下载地址，文件名高亮）
function linkWithTooltip(href, innerHTML, os) {
  var parts = href.split("/");
  var filename = parts[parts.length - 1];
  var tipHTML = parts.slice(0, -1).join("/") + "/" + '<span class="text-ok font-semibold">' + filename + "</span>";
  return '<a href="' + href + '" download onclick="event.stopPropagation()" class="no-underline"' +
    ' data-tip="' + tipHTML.replace(/"/g, "&quot;") + '"' +
    ' onmouseenter="showTooltip(event)" onmouseleave="hideTooltip()">' +
    innerHTML + "</a>";
}

// 官网链接悬停（高亮根域）
function earthTooltip(href) {
  var m = href.match(/^(https?:\/\/[^\/]+)/);
  var domain = m ? m[1] : href;
  return href.replace(domain, '<span class="text-ok font-semibold">' + domain + '</span>');
}

// 纯文本提示
function tipAttr(html) {
  return ' data-tip="' + html.replace(/"/g, "&quot;") + '" onmouseenter="showTooltip(event)" onmouseleave="hideTooltip()"';
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
