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
        <nav class="flex gap-1 items-center">
          ${isIndex ? `
          <button id="btn-read-cache" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">读取缓存</button>
          <button id="btn-check-all" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">检查全部</button>
          <button id="btn-tracker-check" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">检查当前 Tracker</button>
          ` : `
          <button id="btn-sync" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-[#30303b] hover:text-white cursor-pointer border-0 bg-transparent">同步规则</button>
          `}
        </nav>
      </div>
    </div>`;
  if (!isIndex) {
    document.getElementById("btn-sync").addEventListener("click", function () {
      confirmDialog("将从所有规则源同步最新的规则文件", syncRules);
    });
  }
}

// ── 同步规则（可从管理弹窗或别处调用）──
async function syncRules() {
  var ld = showLoading("加载中", "正在同步规则...");
  console.log("[sync]"); var result = await apiPost("/api/rules/sync");
  if (typeof loadSources === "function") loadSources();
  if (result && result.Errors && result.Errors.length) {
    var msg = "同步完成，" + (result.Synced || []).length + " 个更新，" + (result.Skipped || []).length + " 个跳过，" + result.Errors.length + " 个失败<br><br>" +
      result.Errors.map(function(e) { return e.url + ": " + e.reason; }).join("<br>");
    ld.done(msg, true);
  } else if (result && result.Synced) {
    var doneMsg = "同步完成（" + result.Synced.length + " 个更新";
    if (result.Skipped && result.Skipped.length) doneMsg += "，" + result.Skipped.length + " 个跳过";
    doneMsg += "）";
    ld.done(doneMsg);
  } else {
    ld.done("同步完成");
  }
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
  return `<span class="icon-bg inline-flex"><span class="${sz} text-xs text-sub font-semibold inline-flex items-center justify-center" ${tipAttr(label)}>${os.slice(0, 2).toUpperCase()}</span></span>`;
}

function iconImg(file, alt, cls) {
  return `<span class="icon-bg-pink cursor-pointer hover:opacity-80 transition-opacity">${iconImgRaw(file, alt, cls || "w-5 h-5")}</span>`;
}

function iconYes(alt, cls) {
  return `<span class="icon-bg-green cursor-pointer hover:opacity-80 transition-opacity">${iconImgRaw("yes", alt, cls || "w-5 h-5")}</span>`;
}

function iconImgRaw(file, alt, sz) {
  return `<img src="/assets/${file}.svg" class="${sz} inline-block" alt="${alt}" draggable="false">`;
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
    _tooltipEl.className = "fixed bg-[#1c1c22] border border-[rgba(255,255,255,.15)] text-xs text-sub rounded-lg px-3 py-2 shadow-xl pointer-events-none z-[200]";
    _tooltipEl.style.display = "none";
    _tooltipEl.style.opacity = "0";
    _tooltipEl.style.lineHeight = "1.4";
    document.body.appendChild(_tooltipEl);
  }
  return _tooltipEl;
}

function _positionTooltip(e) {
  var tip = _ensureTooltip();
  var maxW = window.innerWidth - 32;
  var rect = e.target.getBoundingClientRect();

  // 先隐藏，测量自然宽度后再决定是否换行
  tip.style.opacity = "0";
  tip.style.display = "block";
  tip.style.visibility = "hidden";
  tip.style.whiteSpace = "nowrap";
  tip.style.maxWidth = "";
  tip.style.wordBreak = "";
  tip.style.overflowWrap = "";
  tip.style.left = "0px";
  tip.style.top = "0px";
  tip.style.transform = "";
  tip.style.right = "";

  // 测量自然单行宽度
  var naturalW = tip.getBoundingClientRect().width;
  if (naturalW > maxW) {
    tip.style.maxWidth = maxW + "px";
    tip.style.whiteSpace = "normal";
    tip.style.overflowWrap = "break-word";
  }

  // 定位
  var left = rect.left + rect.width / 2;
  var above = true;
  tip.style.left = left + "px";
  tip.style.top = (rect.top - 8) + "px";
  tip.style.transform = "translate(-50%, -100%)";
  tip.style.visibility = "";

  var tipRect = tip.getBoundingClientRect();
  // 顶部溢出：翻转到元素下方
  if (tipRect.top < 16) {
    tip.style.top = (rect.bottom + 8) + "px";
    tip.style.transform = "translate(-50%, 0)";
    above = false;
    tipRect = tip.getBoundingClientRect();
  }
  if (tipRect.right > window.innerWidth - 16) {
    tip.style.left = "auto";
    tip.style.right = "16px";
    tip.style.transform = above ? "translate(0, -100%)" : "translate(0, 0)";
  }
  if (tipRect.left < 16) {
    tip.style.left = "16px";
    tip.style.transform = above ? "translate(0, -100%)" : "translate(0, 0)";
  }

  tip.style.opacity = "1";
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
function isDirectDownload(href) {
  return /\.(exe|zip|dmg|pkg|msi|apk|deb|rpm|AppImage|tar\\.gz|tar\\.xz|7z|rar)$/i.test(href);
}

function linkWithTooltip(href, innerHTML, os) {
  var parts = href.split("/");
  var filename = parts[parts.length - 1];
  var tipHTML = parts.slice(0, -1).join("/") + "/" + '<span class="text-ok font-semibold">' + filename + "</span>";
  var attrs = isDirectDownload(href) ? 'download' : 'target="_blank"';
  return '<a href="' + href + '" ' + attrs + ' onclick="event.stopPropagation()" class="no-underline"' +
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

// ── 外部链接弹窗（桌面壳内无法拉起浏览器时展示）──
function openExternalUrl(url) {
  var escaped = url.replace(/\\/g, "\\\\").replace(/'/g, "\\'");
  showModal(
    '<div class="text-sm font-semibold mb-3">外部地址</div>' +
    '<p class="text-text text-xs break-all bg-[#1d1d1d] rounded-lg px-3 py-2 border border-[rgba(255,255,255,.08)] mb-4 leading-relaxed">' + url + '</p>' +
    '<div class="flex justify-end gap-2">' +
    '<button onclick="this.closest(\'.fixed\').remove()" class="px-3 py-1 rounded-lg border border-[rgba(255,255,255,.12)] bg-transparent text-sub text-xs cursor-pointer hover:text-white">取消</button>' +
    '<button onclick="var s=this;navigator.clipboard.writeText(\'' + escaped + '\');s.textContent=\'已复制\';setTimeout(function(){s.textContent=\'复制链接\'},1500)" class="px-3 py-1 rounded-lg border border-[rgba(255,255,255,.12)] bg-transparent text-sub text-xs cursor-pointer hover:text-white">复制链接</button>' +
    '<button onclick="var el=this.closest(\'.fixed\');apiPost(\'/api/open-url\',{url:\'' + escaped + '\'});el.remove()" class="px-3 py-1 rounded-lg bg-[#14b8a6] text-white text-xs font-semibold cursor-pointer hover:opacity-90">确认</button>' +
    '</div>'
  );
}

// ── 确认弹窗 ──
function confirmDialog(msg, cb) {
  showModal(
    '<p class="text-sm mb-4 leading-relaxed">' + msg + '</p>' +
    '<div class="flex justify-end gap-2">' +
    '<button onclick="this.closest(\'.fixed\').remove()" class="px-3 py-1 rounded-lg border border-[rgba(255,255,255,.12)] bg-transparent text-sub text-xs cursor-pointer hover:text-white">取消</button>' +
    '<button id="btn-confirm-exec" class="px-3 py-1 rounded-lg bg-[#14b8a6] text-white text-xs font-semibold cursor-pointer hover:opacity-90">确认</button>' +
    '</div>'
  );
  document.getElementById("btn-confirm-exec").onclick = function () {
    this.closest(".fixed").remove();
    cb();
  };
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
