// Serein shared JS — API 封装、顶栏、公共函数
var SEREIN_DOWNLOADER = "__DL__";
var SEREIN_DOWNLOADER_TYPE = "__DL_TYPE__";
var DOWNLOAD_EXTS = /(?!)/;
const API = window.location.origin;

// HTML 文本转义：把动态文本安全地拼进 innerHTML
function escapeHtml(s) {
    if (s == null) return "";
    return String(s)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#39;");
}
// 属性值转义：只转义 & 与 "，保留 < > 以便 data-tip 这类承载 HTML 的属性
function escapeAttr(s) {
    if (s == null) return "";
    return String(s).replace(/&/g, "&amp;").replace(/"/g, "&quot;");
}
window.escapeHtml = escapeHtml;
window.escapeAttr = escapeAttr;

// 桌面壳（Tauri）由初始化脚本注入；浏览器访问则无
function isDesktop() {
    return (
        typeof window.__SEREIN_DESKTOP__ !== "undefined" &&
        !!window.__SEREIN_DESKTOP__
    );
}
// 是否从本机访问（回环地址）
function isLocalAccess() {
    var h = location.hostname;
    return (
        h === "127.0.0.1" || h === "localhost" || h === "::1" || h === "[::1]"
    );
}
// 是否配置了外部下载器（ndm / 自定义命令）
function hasDownloader() {
    return (
        SEREIN_DOWNLOADER_TYPE === "ndm" || SEREIN_DOWNLOADER_TYPE === "custom"
    );
}
// 补全相对路径为绝对 URL（proxy_url 为相对路径，端点只认绝对地址）
function absoluteUrl(u) {
    try {
        return new URL(u, location.origin).href;
    } catch (e) {
        return u;
    }
}
// 打开链接：桌面壳经服务端拉起系统浏览器；浏览器里直接开新标签
function openUrl(url) {
    if (!url) return;
    url = absoluteUrl(url);
    if (isDesktop()) {
        apiPost("/api/open-url", { url: url });
    } else {
        window.open(url, "_blank");
    }
}

// 主题：localStorage > 系统偏好
(function () {
    var saved = localStorage.getItem("theme");
    function applyBySystem() {
        return window.matchMedia("(prefers-color-scheme: light)").matches;
    }
    function setTheme(theme) {
        var isLight =
            theme === "light" || (theme !== "dark" && applyBySystem());
        document.documentElement.classList.toggle("light", isLight);
    }
    setTheme(saved || "auto");
    window
        .matchMedia("(prefers-color-scheme: light)")
        .addEventListener("change", function () {
            var current = localStorage.getItem("theme") || "auto";
            if (current === "auto") setTheme("auto");
        });
    window.setThemePreference = function (theme) {
        localStorage.setItem("theme", theme);
        setTheme(theme);
    };
    window.getThemePreference = function () {
        return localStorage.getItem("theme") || "auto";
    };
})();

fetch(API + "/api/config")
    .then(function (r) {
        return r.json();
    })
    .then(function (d) {
        if (
            d.profile &&
            d.profile.known_extensions &&
            d.profile.known_extensions.length
        ) {
            var escaped = d.profile.known_extensions.map(function (e) {
                return e.replace(/[.*+?^${}()|[\]\\]/g, function (m) {
                    return "\\" + m;
                });
            });
            DOWNLOAD_EXTS = new RegExp("\\.(" + escaped.join("|") + ")$", "i");
        }
    })
    .catch(function () {});

function compareVersions(a, b) {
    if (!a && !b) return 0;
    if (!a) return -1;
    if (!b) return 1;
    var pa = a.split(".");
    var pb = b.split(".");
    var len = Math.max(pa.length, pb.length);
    for (var i = 0; i < len; i++) {
        var na = parseInt(pa[i]) || 0;
        var nb = parseInt(pb[i]) || 0;
        if (na !== nb) return na > nb ? 1 : -1;
    }
    return 0;
}
window.compareVersions = compareVersions;
window.tipAttr = tipAttr;
window.linkWithTooltip = linkWithTooltip;
window.closeModal = closeModal;
window.confirmDialog = confirmDialog;

// API
async function api(path) {
    const r = await fetch(API + path);
    return _handleResponse(r);
}
async function apiPost(path, body, signal) {
    const r = await fetch(API + path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: body ? JSON.stringify(body) : undefined,
        signal: signal
    });
    return _handleResponse(r);
}
// 统一处理响应：非 2xx 弹一次错误提示
async function _handleResponse(r) {
    var data = null;
    try {
        data = await r.json();
    } catch (e) {
        data = null;
    }
    if (!r.ok) {
        var msg = (data && data.error) || "请求失败 (" + r.status + ")";
        _makeToast("错误", escapeHtml(msg), "bg-err", "bg-err/80", 0);
    }
    return data;
}

// 规则状态：{ message, level } → 渲染信息
// level: ""（无等级）| warn | error | removed
function statusInfo(status) {
    if (!status || !status.message) return null;
    var level = status.level || "";
    var dot = "bg-warn";
    var nameClass = "";
    if (level === "error") {
        dot = "bg-err";
    } else if (level === "removed") {
        dot = "bg-err";
        nameClass = "line-through";
    }
    return {
        message: status.message,
        level: level,
        nameClass: nameClass,
        dotHTML:
            '<span class="inline-block w-2 h-2 rounded-full ' +
            dot +
            ' align-middle"></span>'
    };
}

// 检查完成后，为"带等级"的规则状态弹 toast（无等级不弹）
function showStatusToasts(detail) {
    (detail || []).forEach(function (d) {
        var st = statusInfo(d.status);
        if (!st || !st.level) return;
        var bg = st.level === "warn" ? "bg-warn" : "bg-err";
        _makeToast(
            d.name || d.app_id,
            escapeHtml(st.message),
            bg,
            bg + "/80",
            st.level === "warn" ? 5 : 0
        );
    });
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
    const isSettings = path === "/settings" || path === "/settings.html";
    const isRules = path.startsWith("/rules");

    tb.innerHTML = `
    <div class="bg-bg border-b border-bord-light">
      <div class="max-w-[1200px] mx-auto px-5 flex items-center gap-3 h-12 relative">
        <h1 class="text-lg font-bold cursor-pointer shrink-0" onclick="location.href='/'">Serein</h1>
        <nav class="flex gap-1 ml-2 items-center">
          <a href="/" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/")}">应用</a>
          <a href="/rules" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/rules")}">规则</a>
          <a href="/search" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/search")}">搜索</a>
          <a href="/settings" class="no-underline px-3 py-1.5 rounded-lg text-sm ${navCls("/settings")}">设置</a>
        </nav>
        <div class="flex-1"></div>
        <nav class="flex gap-1 items-center">
          ${
              isIndex
                  ? `
          <button id="btn-tracker-check" style="display:none" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">检查当前 Tracker</button>
          `
                  : isRules
                    ? `
          <button id="btn-rule-check" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">检查错误</button>
          <button id="btn-sync-profile" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">拉取动态配置</button>
          <button id="btn-sync" class="no-underline px-3 py-1.5 rounded-lg text-sm text-sub hover:bg-active hover:text-text cursor-pointer border-0 bg-transparent">拉取规则</button>
          `
                    : ""
          }
        </nav>
      </div>
    </div>`;
    if (isRules) {
        document
            .getElementById("btn-sync")
            .addEventListener("click", function () {
                confirmDialog("将从所有规则源同步最新的规则文件", syncRules);
            });
        document
            .getElementById("btn-sync-profile")
            .addEventListener("click", function () {
                confirmDialog("将从远端拉取最新的动态配置", syncProfile);
            });
        document
            .getElementById("btn-rule-check")
            .addEventListener("click", checkRuleErrors);
    }
}

// 拉取规则（可从管理弹窗或别处调用）
async function syncRules() {
    console.log("[sync]");
    var res = await apiPost("/api/sync", { type: "rules" });
    if (!res || !res.task_id) {
        return;
    }
    startSyncProgress(res.task_id);
}

// 手动检查规则错误（结果直接展示，不经事件总线）
async function checkRuleErrors() {
    var ld = showLoading("规则检查", "正在检查...");
    try {
        var res = await apiPost("/api/rules/check", {});
        var issues = (res && res.issues) || [];
        if (!issues.length) {
            ld.done("未发现规则错误");
            return;
        }
        var errs = issues.filter(function (i) {
            return i.level === "error";
        }).length;
        var warns = issues.length - errs;
        var html = issues
            .map(function (i) {
                return (
                    '<span class="text-' +
                    (i.level === "error" ? "err" : "warn") +
                    '">' +
                    escapeHtml(i.level) +
                    "</span>: " +
                    escapeHtml(i.message)
                );
            })
            .join("<br>");
        ld.done(html, true, errs + " 个错误 / " + warns + " 个警告");
    } catch (e) {
        ld.done(e.message || "请求失败", true);
    }
}

// 拉取动态配置
async function syncProfile() {
    var controller = new AbortController();
    var pm = showProgressModal("动态配置", function () {
        controller.abort();
    });
    pm.setStatus("正在拉取...");
    try {
        var res = await apiPost(
            "/api/sync",
            { type: "profile" },
            controller.signal
        );
        pm.close();
        if (!res || res.error) return;
        if (res.known_extensions && res.known_extensions.length) {
            var escaped = res.known_extensions.map(function (e) {
                return e.replace(/[.*+?^${}()|[\]\\]/g, function (m) {
                    return "\\" + m;
                });
            });
            DOWNLOAD_EXTS = new RegExp("\\.(" + escaped.join("|") + ")$", "i");
        }
        var msg = res.updated ? "动态配置已更新" : "动态配置已是最新";
        _makeToast("动态配置", msg, "bg-ok", "bg-ok/80", 5);
    } catch (e) {
        pm.close();
        if (e && e.name === "AbortError") {
            _makeToast("已终止", "动态配置未更新", "bg-warn", "bg-warn/80", 5);
            return;
        }
        _makeToast(
            "错误",
            escapeHtml((e && e.message) || "请求失败"),
            "bg-err",
            "bg-err/80",
            0
        );
    }
}

// 图标
function platformLabel(os) {
    const labels = {
        macos: "macOS",
        windows: "Windows",
        linux: "Linux",
        android: "Android",
        ios: "iOS"
    };
    return labels[os] || os;
}

function platformIcon(os, cls) {
    const map = {
        macos: "apple",
        windows: "windows",
        linux: "linux",
        ios: "ios",
        android: "android"
    };
    const name = map[os];
    const label = platformLabel(os);
    const sz = cls || "w-5 h-5";
    if (name)
        return `<span class="icon-bg inline-flex">${iconImgRaw(name, label, sz)}</span>`;
    return `<span class="icon-bg inline-flex"><span class="${sz} text-xs text-sub font-semibold inline-flex items-center justify-center">${os.slice(0, 2).toUpperCase()}</span></span>`;
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
    if (Array.isArray(u))
        return u.length > 1 ? `${u[0]} (+${u.length - 1})` : u[0];
    return "";
}

// 通知组件
var _toastStack = [];

function _repositionToasts() {
    _toastStack = _toastStack.filter(function (t) {
        return !t.closed();
    });
    var bottom = 16;
    for (var i = _toastStack.length - 1; i >= 0; i--) {
        _toastStack[i].el.style.bottom = bottom + "px";
        bottom += _toastStack[i].el.getBoundingClientRect().height + 12;
    }
}

function _makeToast(title, body, titleBg, bodyBg, autoCloseSec) {
    var el = document.createElement("div");
    el.className =
        "fixed left-4 right-4 z-[200] rounded-lg shadow-xl text-sm transition-all duration-300";
    el.style.bottom = "16px";
    el.style.opacity = "0";
    el.style.transform = "translateY(8px)";
    el.style.width = "fit-content";
    el.style.marginLeft = "auto";
    el.innerHTML =
        '<div class="' +
        titleBg +
        ' text-white font-semibold px-4 py-2 rounded-t-lg flex items-center justify-between">' +
        "<span>" +
        escapeHtml(title) +
        "</span>" +
        '<span class="cursor-pointer text-white opacity-60 hover:opacity-100 text-base leading-none ml-3">✕</span>' +
        "</div>" +
        '<div class="' +
        bodyBg +
        " text-white px-4 py-2 rounded-b-lg" +
        (titleBg === "bg-ok" ? "" : " select-text") +
        '">' +
        (body || "") +
        "</div>";
    document.body.appendChild(el);
    requestAnimationFrame(function () {
        el.style.opacity = "1";
        el.style.transform = "translateY(0)";
    });

    var timer = null;
    var closed = false;
    function close() {
        if (closed) return;
        closed = true;
        if (timer) clearTimeout(timer);
        el.style.opacity = "0";
        el.style.transform = "translateY(8px)";
        _repositionToasts();
        setTimeout(function () {
            el.remove();
            _repositionToasts();
        }, 300);
    }

    el.querySelector("span[class*='cursor-pointer']").onclick = close;
    // 成功 toast 可整块点击关闭
    if (titleBg === "bg-ok") {
        el.style.cursor = "pointer";
        el.onclick = close;
    }

    if (autoCloseSec > 0) {
        timer = setTimeout(close, autoCloseSec * 1000);
    }

    _toastStack.push({
        el: el,
        closed: function () {
            return closed;
        }
    });
    _repositionToasts();

    return {
        el: el,
        close: close,
        done: function (okBody, isError, titleText) {
            if (closed) {
                var t = _makeToast(
                    isError ? "错误" : "成功",
                    okBody,
                    isError ? "bg-err" : "bg-ok",
                    isError ? "bg-err/80" : "bg-ok/80",
                    isError ? 0 : 5
                );
                if (titleText)
                    t.el.querySelector("span:first-child").textContent =
                        titleText;
                return;
            }
            var tb = isError ? "bg-err" : "bg-ok";
            var bb = isError ? "bg-err/80" : "bg-ok/80";
            var tt = titleText || (isError ? "错误" : "成功");
            el.querySelector("div:first-child").className =
                tb +
                " text-white font-semibold px-4 py-2 rounded-t-lg flex items-center justify-between";
            el.querySelector("span:first-child").textContent = tt;
            el.querySelector("div:last-child").className =
                bb +
                " text-white px-4 py-2 rounded-b-lg" +
                (isError ? " select-text" : "");
            el.querySelector("div:last-child").innerHTML = okBody || "";
            if (isError) {
                if (timer) clearTimeout(timer);
            } else {
                if (timer) clearTimeout(timer);
                timer = setTimeout(close, 5000);
                el.style.cursor = "pointer";
                el.onclick = close;
            }
        }
    };
}

function showLoading(title, body) {
    return _makeToast(
        title || "加载中",
        body || "",
        "bg-warn",
        "bg-warn/80",
        0
    );
}

// 通用悬停提示（全局单例）
var _tooltipEl = null;
function _ensureTooltip() {
    if (!_tooltipEl) {
        _tooltipEl = document.createElement("div");
        _tooltipEl.className =
            "fixed bg-surface-raised border border-bord-strong text-xs text-sub rounded-lg px-3 py-2 shadow-xl pointer-events-none z-[200] transition-opacity duration-150";
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
    tip.style.top = rect.top - 8 + "px";
    tip.style.transform = "translate(-50%, -100%)";
    tip.style.visibility = "";

    var tipRect = tip.getBoundingClientRect();
    // 顶部溢出：翻转到元素下方
    if (tipRect.top < 16) {
        tip.style.top = rect.bottom + 8 + "px";
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
    if (_tooltipEl) {
        _tooltipEl.style.opacity = "0";
    }
}

// 下载链接白名单
function isDirectDownload(href) {
    return DOWNLOAD_EXTS.test(href);
}

async function downloadFile(url) {
    url = absoluteUrl(url);
    var ld = showLoading("下载", "正在发送到下载器...");
    try {
        var res = await apiPost("/api/download", { url: url });
        ld.done(
            res.message || "",
            !res || res.status === "error",
            res.status === "error" ? "下载失败" : "下载"
        );
    } catch (e) {
        ld.done(e.message || "请求失败", true);
    }
}

function linkWithTooltip(href, innerHTML, os, opts) {
    opts = opts || {};
    var parts = href.split("/");
    var filename = parts[parts.length - 1];
    var tipHTML =
        escapeHtml(parts.slice(0, -1).join("/")) +
        "/" +
        '<span class="text-ok font-semibold">' +
        escapeHtml(filename) +
        "</span>";
    var hrefAttr = escapeAttr(href);
    var dl = opts.proxy || href;
    var attrs =
        ' href="' +
        hrefAttr +
        '" data-url="' +
        hrefAttr +
        '" data-dl="' +
        escapeAttr(dl) +
        '" class="no-underline" data-tip="' +
        escapeAttr(tipHTML) +
        '" onmouseenter="showTooltip(event)" onmouseleave="hideTooltip()"';
    var method = opts.method || "";
    var canDownload = isLocalAccess() && hasDownloader();
    var action;
    if (method === "browser") {
        action = "openUrl(this.dataset.dl)";
    } else if (method === "downloader") {
        action = canDownload
            ? "downloadFile(this.dataset.dl)"
            : "openUrl(this.dataset.dl)";
    } else if (os === "msvsix" || os === "openvsx" || isDirectDownload(href)) {
        action = canDownload
            ? "downloadFile(this.dataset.dl)"
            : "openUrl(this.dataset.dl)";
    } else {
        action = "openDownloadPage(this.dataset.dl)";
    }
    return (
        "<a" +
        attrs +
        ' onclick="event.stopPropagation();event.preventDefault();' +
        action +
        '">' +
        innerHTML +
        "</a>"
    );
}

// 链接弹窗：官网 + 各平台下载链接（含代理链接）
function openLinksModal(appId, site) {
    var r =
        typeof checkResults !== "undefined" && checkResults
            ? checkResults[appId]
            : null;
    if (!site && r) site = r.official_website;

    var items = [];
    var notes = [];
    if (site) {
        items.push({ label: "官网", url: site });
    } else {
        notes.push("无官网");
    }

    if (!r || !r.platforms || Object.keys(r.platforms).length === 0) {
        notes.push("尚未检查更新，暂无下载地址");
    } else {
        var order = ["macos", "windows", "linux", "ios", "android"];
        var oses = Object.keys(r.platforms).sort(function (a, b) {
            return order.indexOf(a) - order.indexOf(b);
        });
        var noLink = [];
        var any = false;
        oses.forEach(function (os) {
            var p = r.platforms[os];
            var urls = typeof p.url === "string" ? [p.url] : p.url || [];
            var proxies =
                typeof p.proxy_url === "string"
                    ? [p.proxy_url]
                    : p.proxy_url || [];
            if (urls.length === 0) {
                noLink.push(os);
                return;
            }
            urls.forEach(function (u, i) {
                any = true;
                items.push({ label: os, url: u });
                if (proxies[i])
                    items.push({ label: os + " · 代理", url: proxies[i] });
            });
        });
        if (!any) notes.push("无下载地址");
        else if (noLink.length > 0)
            notes.push("无下载地址：" + noLink.join("、"));
    }

    var body = items.map(linkItem).join("");
    body += notes
        .map(function (n) {
            return (
                '<div class="text-xs text-sub px-1 py-1.5">' +
                escapeHtml(n) +
                "</div>"
            );
        })
        .join("");

    showModal(
        '<div class="flex items-center justify-between mb-3">' +
            '<div class="text-base font-bold">官网/下载地址</div>' +
            '<button onclick="closeModal(this.closest(\'.fixed\'))" class="w-7 h-7 flex items-center justify-center rounded-lg border border-bord bg-transparent text-sub cursor-pointer hover:bg-active hover:text-text">&times;</button>' +
            "</div>" +
            '<div class="max-h-[70vh] overflow-y-auto -mr-2 pr-2">' +
            body +
            "</div>",
        true
    );
}

function linkItem(it) {
    var abs = escapeAttr(absoluteUrl(it.url));
    var cls =
        'class="px-2.5 py-1 rounded-md border border-bord bg-transparent text-sub text-xs cursor-pointer hover:bg-active hover:text-text"';
    return (
        '<div class="mb-5">' +
        '<div class="flex items-center justify-between mb-1.5">' +
        '<div class="text-sm text-text font-medium">' +
        escapeHtml(it.label) +
        "</div>" +
        '<div class="flex gap-2">' +
        '<button data-url="' +
        abs +
        '" onclick="copyLink(this)" ' +
        cls +
        ">复制</button>" +
        '<button data-url="' +
        abs +
        '" onclick="openUrl(this.dataset.url)" ' +
        cls +
        ">打开</button>" +
        "</div>" +
        "</div>" +
        '<input value="' +
        abs +
        '" spellcheck="false" class="w-full bg-bg border border-bord-mid rounded-lg px-3 py-2.5 text-sm text-text outline-none focus:border-accent overflow-x-auto whitespace-nowrap">' +
        "</div>"
    );
}

function copyLink(btn) {
    var done = function () {
        btn.textContent = "已复制";
        setTimeout(function () {
            btn.textContent = "复制";
        }, 1500);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(btn.dataset.url).then(done, done);
    } else {
        done();
    }
}

// 纯文本提示
function tipAttr(html) {
    return (
        ' data-tip="' +
        escapeAttr(html) +
        '" onmouseenter="showTooltip(event)" onmouseleave="hideTooltip()"'
    );
}

// 外部链接弹窗（桌面壳内无法拉起浏览器时展示）
// 非白名单下载链接弹窗
function openDownloadPage(url) {
    var hasDL = isLocalAccess() && hasDownloader();
    var urlAttr = escapeAttr(url);
    var row1 =
        '<button onclick="closeModal(this.closest(\'.fixed\'))" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">取消</button>' +
        '<button data-url="' +
        urlAttr +
        '" onclick="var s=this;navigator.clipboard.writeText(this.dataset.url);s.textContent=\'已复制\';setTimeout(function(){s.textContent=\'复制链接\'},1500)" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">复制链接</button>' +
        '<button data-url="' +
        urlAttr +
        '" onclick="openUrl(this.dataset.url);closeModal(this.closest(\'.fixed\'))" class="flex-1 px-4 py-2 rounded-lg bg-accent text-white text-sm font-semibold cursor-pointer hover:opacity-90">打开</button>';
    var row2 = hasDL
        ? '<button data-url="' +
          urlAttr +
          '" onclick="downloadFile(this.dataset.url);closeModal(this.closest(\'.fixed\'))" class="w-full px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">仍然发送到下载器</button>'
        : "";
    showModal(
        '<div class="text-base font-bold mb-3">外部地址</div>' +
            '<p class="text-text text-sm mb-3 leading-relaxed">此链接看起来不是一个常见的文件，或许是一个网页而非安装包。<br />是否要在外部浏览器打开？</p>' +
            '<p class="select-text text-text text-xs break-all bg-bg rounded-lg px-3 py-2 border border-bord-mid mb-4 leading-relaxed">' +
            escapeHtml(url) +
            "</p>" +
            '<div class="flex gap-2 mb-2">' +
            row1 +
            "</div>" +
            (row2 ? row2 : "")
    );
}

function openExternalUrl(url) {
    var urlAttr = escapeAttr(url);
    showModal(
        '<div class="text-base font-bold mb-3">外部地址</div>' +
            '<p class="text-text text-sm mb-3 leading-relaxed">将会在浏览器中打开此链接。</p>' +
            '<p class="select-text text-text text-xs break-all bg-bg rounded-lg px-3 py-2 border border-bord-mid mb-4 leading-relaxed">' +
            escapeHtml(url) +
            "</p>" +
            '<div class="flex gap-2">' +
            '<button onclick="closeModal(this.closest(\'.fixed\'))" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">取消</button>' +
            '<button data-url="' +
            urlAttr +
            '" onclick="var s=this;navigator.clipboard.writeText(this.dataset.url);s.textContent=\'已复制\';setTimeout(function(){s.textContent=\'复制链接\'},1500)" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">复制链接</button>' +
            '<button data-url="' +
            urlAttr +
            '" onclick="openUrl(this.dataset.url);closeModal(this.closest(\'.fixed\'))" class="flex-1 px-4 py-2 rounded-lg bg-accent text-white text-sm font-semibold cursor-pointer hover:opacity-90">打开</button>' +
            "</div>"
    );
}

// 确认弹窗
function confirmDialog(msg, cb) {
    showModal(
        '<p class="text-sm mb-4 leading-relaxed">' +
            escapeHtml(msg) +
            "</p>" +
            '<div class="flex gap-2">' +
            '<button onclick="closeModal(this.closest(\'.fixed\'))" class="flex-1 px-4 py-2 rounded-lg border border-bord bg-transparent text-sub text-sm cursor-pointer hover:bg-active hover:text-text">取消</button>' +
            '<button id="btn-confirm-exec" class="flex-1 px-4 py-2 rounded-lg bg-accent text-white text-sm font-semibold cursor-pointer hover:opacity-90">确认</button>' +
            "</div>"
    );
    document.getElementById("btn-confirm-exec").onclick = function () {
        closeModal(this.closest(".fixed"));
        cb();
    };
}

// 弹窗
function showModal(html, wide) {
    const el = document.createElement("div");
    el.className =
        "fixed inset-0 z-[51] flex items-center justify-center bg-overlay transition-opacity duration-200";
    el.style.opacity = "0";
    el.onclick = (e) => {
        if (e.target === el) el.remove();
    };
    const size = wide
        ? "min-w-[560px] max-w-[780px]"
        : "min-w-[400px] max-w-[520px]";
    el.innerHTML = `<div class="bg-surface-alt border border-bord rounded-xl p-6 ${size} shadow-2xl transition-all duration-200" style="opacity:0;transform:scale(.95)">${html}</div>`;
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
    setTimeout(function () {
        el.remove();
    }, 200);
}

// 视图切换动画
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
    if (showEl)
        setTimeout(
            function () {
                showView(showEl);
            },
            hideEl ? 60 : 0
        );
}

// 原地刷新：淡出 → 回调 → 淡入（总耗时尽量短）
function refreshView(el, updateFn) {
    if (!el || el.classList.contains("hidden")) {
        updateFn();
        showView(el);
        return;
    }
    el.style.transition = "opacity .1s ease";
    el.style.opacity = "0";
    setTimeout(function () {
        updateFn();
        el.style.opacity = "1";
        setTimeout(function () {
            el.style.transition = "";
        }, 100);
    }, 80);
}

// 进度弹窗（全屏遮罩，不可关闭）

function showProgressModal(title, cancel) {
    var overlay = document.createElement("div");
    overlay.className =
        "fixed inset-0 z-[200] bg-overlay flex items-center justify-center transition-opacity duration-200";
    overlay.style.opacity = "0";
    overlay.id = "progress-overlay";

    var card = document.createElement("div");
    card.className =
        "bg-surface-alt border border-bord rounded-xl p-6 w-[440px] max-w-[90vw] shadow-2xl transition-all duration-200";
    card.style.opacity = "0";
    card.style.transform = "scale(.95)";
    card.innerHTML =
        '<div class="flex items-center justify-between mb-4">' +
        '<h3 id="prog-title" class="text-base font-bold text-text">' +
        escapeHtml(title) +
        "</h3>" +
        '<button id="prog-cancel" class="px-3 py-1 rounded-lg border border-[rgba(255,0,0,.3)] bg-transparent text-[#dc2626] text-xs cursor-pointer hover:bg-[rgba(255,0,0,.1)]">终止</button>' +
        "</div>" +
        '<div class="bg-bg rounded-full h-2 mb-3 overflow-hidden">' +
        '<div id="prog-bar" class="bg-accent h-full rounded-full transition-all duration-300" style="width:0%"></div>' +
        "</div>" +
        '<p id="prog-status" class="text-sub text-sm">准备中...</p>';
    overlay.appendChild(card);
    document.body.appendChild(overlay);
    requestAnimationFrame(function () {
        overlay.style.opacity = "1";
        card.style.opacity = "1";
        card.style.transform = "scale(1)";
    });

    // 终止：cancel 为 URL 字符串（POST）或回调函数
    if (cancel) {
        card.querySelector("#prog-cancel").onclick = function () {
            if (typeof cancel === "function") cancel();
            else fetch(cancel, { method: "POST" });
            var bar = card.querySelector("#prog-bar");
            bar.style.background = "var(--c-warn)";
            card.querySelector("#prog-title").textContent =
                title + " - 正在终止...";
            card.querySelector("#prog-status").textContent = "";
            card.querySelector("#prog-cancel").remove();
        };
    }

    return {
        setProgress: function (done, total) {
            var pct = total > 0 ? Math.round((done / total) * 100) : 0;
            card.querySelector("#prog-title").textContent =
                title + "（" + done + "/" + total + "）";
            card.querySelector("#prog-bar").style.width = pct + "%";
        },
        setStatus: function (text) {
            card.querySelector("#prog-status").textContent = text || "";
        },
        close: function () {
            overlay.style.opacity = "0";
            card.style.opacity = "0";
            card.style.transform = "scale(.95)";
            setTimeout(function () {
                overlay.remove();
            }, 200);
        }
    };
}

// SSE 进度检查（批量）
async function asyncCheck(apiPath, body, onDone) {
    var res = await apiPost(apiPath, body);
    if (!res || !res.task_id) {
        return;
    }
    var total = parseInt(res.total) || 0;
    var pm = showProgressModal(
        "检查更新",
        API + "/api/check/cancel/" + res.task_id
    );
    var evt = new EventSource(API + "/api/progress/" + res.task_id);
    evt.onmessage = function (e) {
        var d = JSON.parse(e.data);
        if (d.step === "done") {
            evt.close();
            pm.close();
            if (d.cancelled) {
                _makeToast("已终止", "取消检查", "bg-warn", "bg-warn/80", 5);
            }
            onDone(d.cancelled);
            return;
        }
        if (d.step === "app") {
            pm.setProgress(d.done, d.total);
            pm.setStatus(d.name);
        }
    };
    evt.onerror = function () {
        evt.close();
        pm.close();
        onDone();
    };
}

// SSE 进度拉取规则
function startSyncProgress(taskId) {
    var sourcesSkipped = 0,
        sourcesUpdated = 0,
        sourcesFailed = 0,
        filesTotal = 0,
        fileErrors = 0;
    var pm = showProgressModal("拉取规则", API + "/api/check/cancel/" + taskId);
    var evt = new EventSource(API + "/api/progress/" + taskId);
    evt.onmessage = function (e) {
        var d = JSON.parse(e.data);
        if (d.step === "done") {
            evt.close();
            pm.close();
            var parts = [];
            if (d.sources_skipped > 0)
                parts.push(d.sources_skipped + " 个源已是最新，跳过");
            if (d.sources_updated > 0)
                parts.push(
                    d.sources_updated + " 个源已更新，共 " + d.files + " 条规则"
                );
            else if (d.sources_updated == 0 && d.sources_total > 0)
                parts.push("无任何规则源需要更新");
            var failures = d.failures || [];
            var failedSources = d.sources_failed || 0;
            if (failures.length > 0) {
                var lines = [];
                for (var i = 0; i < failures.length && i < 20; i++) {
                    var f = failures[i];
                    lines.push(
                        escapeHtml(f.source) +
                            (f.file ? "/" + escapeHtml(f.file) : "") +
                            "：" +
                            escapeHtml(f.error)
                    );
                }
                if (failures.length > 20)
                    lines.push("...还有 " + (failures.length - 20) + " 条");
                parts.push(
                    failedSources + " 个源更新失败：<br>" + lines.join("<br>")
                );
            } else if (d.file_errors > 0) {
                parts.push(d.file_errors + " 条规则下载失败");
            }
            if (d.cancelled) {
                _makeToast("已终止", "取消拉取，规则未被替换", "bg-warn", "bg-warn/80", 5);
                if (typeof loadSources === "function") loadSources();
                return;
            }
            var msg = parts.length > 0 ? parts.join("<br>") : "同步完成";
            var hasError =
                d.file_errors > 0 || failedSources > 0 || sourcesFailed > 0;
            showLoading("拉取规则", msg).done(msg, hasError);
            if (typeof loadSources === "function") loadSources();
            return;
        }
        if (d.step === "error") {
            sourcesFailed++;
            pm.setStatus(d.name + " 获取失败");
            return;
        }
        if (d.step === "list") {
            pm.setStatus("正在拉取规则源 " + d.name);
            return;
        }
        if (d.step === "skip") {
            sourcesSkipped++;
            pm.setStatus(d.name + "（已是最新，跳过）");
            return;
        }
        if (d.step === "source") {
            sourcesUpdated++;
            pm.setStatus(d.name + " 开始拉取");
            return;
        }
        if (d.step === "file") {
            if (d.done) pm.setProgress(d.done, d.total);
            if (d.name.indexOf("(失败)") >= 0) {
                fileErrors++;
            }
            pm.setStatus(d.name);
        }
        if (d.step === "start" && d.total) {
            pm.setProgress(0, d.total);
        }
    };
    evt.onerror = function () {
        evt.close();
        pm.close();
        if (typeof loadSources === "function") loadSources();
    };
}

(function () {
    var SEEN_KEY = "serein_seen_events";
    var seen = {};
    try {
        seen = JSON.parse(sessionStorage.getItem(SEEN_KEY) || "{}") || {};
    } catch (e) {
        seen = {};
    }
    function remember(id) {
        seen[id] = 1;
        var keys = Object.keys(seen);
        for (var i = 0; i < keys.length - 200; i++) delete seen[keys[i]];
        try {
            sessionStorage.setItem(SEEN_KEY, JSON.stringify(seen));
        } catch (e) {}
    }
    function connectEvents() {
        var es = new EventSource(API + "/api/events");
        es.onmessage = function (e) {
            var d = JSON.parse(e.data);
            // 已处理过的事件（含历史回放）不再弹出；跨翻页/刷新保留
            if (d.id) {
                if (seen[d.id]) return;
                remember(d.id);
            }
            if (d.level === "error") {
                _makeToast(
                    "错误: " + (d.context || "后端"),
                    escapeHtml(d.message),
                    "bg-err",
                    "bg-err/80",
                    0
                );
            } else if (d.level === "warn") {
                _makeToast(
                    "警告: " + (d.context || "后端"),
                    escapeHtml(d.message),
                    "bg-warn",
                    "bg-warn/80",
                    8
                );
            }
        };
        es.onerror = function () {
            es.close();
            setTimeout(connectEvents, 3000);
        };
    }
    connectEvents();
})();

// 全局 "/" 跳转到搜索页（搜索页内自行聚焦输入框）
document.addEventListener("keydown", function (e) {
    if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
    var t = e.target;
    if (t) {
        var tag = t.tagName;
        if (
            tag === "INPUT" ||
            tag === "TEXTAREA" ||
            tag === "SELECT" ||
            t.isContentEditable
        )
            return;
    }
    var p = location.pathname;
    if (p === "/search" || p === "/search.html") return;
    e.preventDefault();
    location.href = "/search";
});
