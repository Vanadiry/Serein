// Serein shared JS — API 封装、顶栏、公共函数
const API = window.location.origin;

// 主题：跟随系统
(function () {
  var mq = window.matchMedia("(prefers-color-scheme: light)");
  function apply(e) { document.documentElement.classList.toggle("light", e.matches); }
  apply(mq);
  mq.addEventListener("change", apply);
})();

// API
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

// 顶栏
function renderTopbar(current) {
  const tb = document.getElementById("topbar");
  if (!tb) return;
  const path = location.pathname;
  tb.style.cssText = "position:sticky;top:0;z-index:50";

  function navCls(href) {
    if (href === "/" && path === "/") return "bg-active text-text";
    if (href !== "/" && path.startsWith(href)) return "bg-active text-text";
    return "text-sub hover:bg-active hover:text-text";
  }

  const isIndex = path === "/" || path === "/index.html" || path === "";

  tb.innerHTML = `
    <div class="bg-bg border-b border-bord-light">
      <div class="max-w-[1200px] mx-auto px-5 flex items-center gap-3 h-12 relative">
        <h1 class="text-lg font-bold cursor-pointer shrink-0" onclick="location.href='/'">Serein</h1>
        <nav class="flex gap-1 ml-2 items-center">
          <a href="/" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/")}">应用</a>
          <a href="/rules" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/rules")}">规则</a>
        </nav>
        <div class="flex-1"></div>
        <nav class="flex gap-1 items-center">
          ${isIndex ? `
          <button id="btn-read-cache" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">读取缓存</button>
          <button id="btn-check-all" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">检查全部</button>
          <button id="btn-tracker-check" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">检查当前 Tracker</button>
          ` : `
          <button id="btn-sync" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">同步规则</button>
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

// 同步规则（可从管理弹窗或别处调用）
async function syncRules() {
  console.log("[sync]");
  var res = await apiPost("/api/rules/sync");
  if (!res || !res.task_id) { return; }
  startSyncProgress(res.task_id);
}

// 图标
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

// 工具
function badge(text, bg, fg) {
  return `<span class="${bg} ${fg} px-1.5 py-px rounded text-[10px] font-semibold">${text}</span>`;
}

function formatURL(u) {
  if (!u) return "";
  if (typeof u === "string") return u;
  if (Array.isArray(u)) return u.length > 1 ? `${u[0]} (+${u.length - 1})` : u[0];
  return "";
}

// 通知组件
var _currentToast = null;

function _makeToast(title, body, titleBg, bodyBg, autoCloseSec) {
  if (_currentToast) {
    var old = _currentToast;
    old.style.opacity = "0";
    setTimeout(function() { old.remove(); }, 300);
  }

  var el = document.createElement("div");
  el.className = "fixed bottom-4 left-4 right-4 z-[100] rounded-lg shadow-xl text-sm transition-opacity duration-200";
  el.style.opacity = "0";
  el.style.width = "fit-content";
  el.style.marginLeft = "auto";
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
        var t = _makeToast(isError ? "错误" : "成功", okBody, isError ? "bg-err" : "bg-ok", isError ? "bg-err/80" : "bg-ok/80", isError ? 0 : 5);
        if (titleText) t.el.querySelector("span:first-child").textContent = titleText;
        return;
      }
      var tb = isError ? "bg-err" : "bg-ok";
      var bb = isError ? "bg-err/80" : "bg-ok/80";
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
  return _makeToast(title || "加载中", body || "", "bg-warn", "bg-warn/80", 0);
}

// 通用悬停提示（全局单例）
var _tooltipEl = null;
function _ensureTooltip() {
  if (!_tooltipEl) {
    _tooltipEl = document.createElement("div");
    _tooltipEl.className = "fixed bg-surface-raised border border-bord-strong text-xs text-sub rounded-lg px-3 py-2 shadow-xl pointer-events-none z-[200] transition-opacity duration-150";
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
  return /\.(exe|zip|dmg|pkg|msi|apk|deb|rpm|AppImage|tar\\.gz|tar\\.xz|7z|rar|ipa)$/i.test(href);
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

// 外部链接弹窗（桌面壳内无法拉起浏览器时展示）
function openExternalUrl(url) {
  var escaped = url.replace(/\\/g, "\\\\").replace(/'/g, "\\'");
  showModal(
    '<div class="text-sm font-semibold mb-3">外部地址</div>' +
    '<p class="select-text text-text text-xs break-all bg-bg rounded-lg px-3 py-2 border border-bord-mid mb-4 leading-relaxed">' + url + '</p>' +
    '<div class="flex gap-2">' +
    '<button onclick="closeModal(this.closest(\'.fixed\'))" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">取消</button>' +
    '<button onclick="var s=this;navigator.clipboard.writeText(\'' + escaped + '\');s.textContent=\'已复制\';setTimeout(function(){s.textContent=\'复制链接\'},1500)" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">复制链接</button>' +
    '<button onclick="var el=this.closest(\'.fixed\');apiPost(\'/api/open-url\',{url:\'' + escaped + '\'});closeModal(el)" class="flex-1 px-4 py-2 rounded-lg bg-accent text-white text-sm font-semibold cursor-pointer hover:opacity-90">确认</button>' +
    '</div>'
  );
}

// 确认弹窗
function confirmDialog(msg, cb) {
  showModal(
    '<p class="text-sm mb-4 leading-relaxed">' + msg + '</p>' +
    '<div class="flex gap-2">' +
    '<button onclick="closeModal(this.closest(\'.fixed\'))" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">取消</button>' +
    '<button id="btn-confirm-exec" class="flex-1 px-4 py-2 rounded-lg bg-accent text-white text-sm font-semibold cursor-pointer hover:opacity-90">确认</button>' +
    '</div>'
  );
  document.getElementById("btn-confirm-exec").onclick = function () {
    closeModal(this.closest(".fixed"));
    cb();
  };
}

// 弹窗
function showModal(html) {
  const el = document.createElement("div");
  el.className = "fixed inset-0 z-50 flex items-center justify-center bg-overlay transition-opacity duration-200";
  el.style.opacity = "0";
  el.onclick = e => { if (e.target === el) el.remove(); };
  el.innerHTML = `<div class="bg-surface-alt border border-bord rounded-xl p-6 min-w-[400px] max-w-[520px] shadow-2xl transition-all duration-200" style="opacity:0;transform:scale(.95)">${html}</div>`;
  document.body.appendChild(el);
  requestAnimationFrame(function () {
    el.style.opacity = "1";
    el.firstElementChild.style.opacity = "1";
    el.firstElementChild.style.transform = "scale(1)";
  });
  return el;
}

function closeModal(el) {
  if (!el) return;
  el.style.opacity = "0";
  if (el.firstElementChild) {
    el.firstElementChild.style.opacity = "0";
    el.firstElementChild.style.transform = "scale(.95)";
  }
  setTimeout(function () { el.remove(); }, 200);
}

// ── 视图切换动画 ──
function showView(el) {
  if (!el || !el.classList.contains("hidden")) return;
  el.classList.remove("hidden");
  el.style.animation = "none";
  el.offsetHeight; // force reflow
  el.style.animation = "";
}

function hideView(el) {
  if (!el || el.classList.contains("hidden")) return;
  el.style.opacity = "0";
  el.style.transition = "opacity .1s ease";
  setTimeout(function () {
    el.classList.add("hidden");
    el.style.opacity = "";
    el.style.transition = "";
  }, 100);
}

function swapView(hideEl, showEl) {
  if (hideEl) hideView(hideEl);
  if (showEl) setTimeout(function () { showView(showEl); }, hideEl ? 60 : 0);
}

// 原地刷新：淡出 → 回调 → 淡入（总耗时尽量短）
function refreshView(el, updateFn) {
  if (!el || el.classList.contains("hidden")) { updateFn(); showView(el); return; }
  el.style.transition = "opacity .1s ease";
  el.style.opacity = "0";
  setTimeout(function () {
    updateFn();
    el.style.opacity = "1";
    setTimeout(function () { el.style.transition = ""; }, 100);
  }, 80);
}

// ── 进度弹窗（全屏遮罩，不可关闭）──

function showProgressModal(title, cancelUrl) {
  var overlay = document.createElement("div");
  overlay.className = "fixed inset-0 z-[200] bg-overlay flex items-center justify-center transition-opacity duration-200";
  overlay.style.opacity = "0";
  overlay.id = "progress-overlay";

  var card = document.createElement("div");
  card.className = "bg-surface-alt border border-bord rounded-xl p-6 w-[440px] max-w-[90vw] shadow-2xl transition-all duration-200";
  card.style.opacity = "0";
  card.style.transform = "scale(.95)";
  card.innerHTML =
    '<div class="flex items-center justify-between mb-4">' +
    '<h3 id="prog-title" class="text-base font-bold text-text">' + title + '</h3>' +
    '<button id="prog-cancel" class="px-3 py-1 rounded-lg border border-[rgba(255,0,0,.3)] bg-transparent text-[#dc2626] text-xs cursor-pointer hover:bg-[rgba(255,0,0,.1)]">终止</button>' +
    '</div>' +
    '<div class="bg-bg rounded-full h-2 mb-3 overflow-hidden">' +
    '<div id="prog-bar" class="bg-accent h-full rounded-full transition-all duration-300" style="width:0%"></div>' +
    '</div>' +
    '<p id="prog-status" class="text-sub text-sm">准备中...</p>';
  overlay.appendChild(card);
  document.body.appendChild(overlay);
  requestAnimationFrame(function () {
    overlay.style.opacity = "1";
    card.style.opacity = "1";
    card.style.transform = "scale(1)";
  });

  // 终止
  if (cancelUrl) {
    card.querySelector("#prog-cancel").onclick = function () {
      fetch(cancelUrl, { method: "POST" });
      var bar = card.querySelector("#prog-bar");
      bar.style.background = "var(--c-warn)";
      card.querySelector("#prog-title").textContent = title + " - 正在终止...";
      card.querySelector("#prog-status").textContent = "";
      card.querySelector("#prog-cancel").remove();
    };
  }

  return {
    setProgress: function (done, total) {
      var pct = total > 0 ? Math.round(done / total * 100) : 0;
      card.querySelector("#prog-title").textContent = title + "（" + done + "/" + total + "）";
      card.querySelector("#prog-bar").style.width = pct + "%";
    },
    setStatus: function (text) {
      card.querySelector("#prog-status").textContent = text || "";
    },
    close: function () {
      overlay.style.opacity = "0";
      card.style.opacity = "0";
      card.style.transform = "scale(.95)";
      setTimeout(function () { overlay.remove(); }, 200);
    }
  };
}

// SSE 进度检查（批量）
async function asyncCheck(apiPath, body, onDone) {
  var res = await apiPost(apiPath, body);
  if (!res || !res.task_id) { return; }
  var total = parseInt(res.total) || 0;
  var pm = showProgressModal("检查更新", API + "/api/check/cancel/" + res.task_id);
  var evt = new EventSource(API + "/api/progress/" + res.task_id);
  evt.onmessage = function (e) {
    var d = JSON.parse(e.data);
    if (d.step === "done") { evt.close(); pm.close(); onDone(); return; }
    if (d.step === "app") { pm.setProgress(d.done, d.total); pm.setStatus(d.name); }
  };
  evt.onerror = function () { evt.close(); pm.close(); onDone(); };
}

// SSE 进度同步规则
function startSyncProgress(taskId) {
  var pm = showProgressModal("同步规则", API + "/api/check/cancel/" + taskId);
  var evt = new EventSource(API + "/api/progress/" + taskId);
  evt.onmessage = function (e) {
    var d = JSON.parse(e.data);
    if (d.step === "done") { evt.close(); pm.close(); showLoading("同步规则", "同步完成").done("同步完成"); if (typeof loadSources === "function") loadSources(); return; }
    if (d.step === "list") { pm.setStatus("正在拉取规则源 " + d.name); return; }
    if (d.step === "file") { pm.setProgress(d.done, d.total); pm.setStatus(d.name); }
  };
  evt.onerror = function () { evt.close(); pm.close(); if (typeof loadSources === "function") loadSources(); };
}
