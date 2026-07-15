/* Route Debugger — admin UI logic. Monochrome, jQuery-based. */

let capturesCache = [];
let selectedId = null;
let currentView = "captures";
let refreshTimer = null;

// ---------- CSRF + API helpers ----------
function csrf() {
    const m = document.querySelector('meta[name="zoraxy.csrf.Token"]');
    return m ? m.getAttribute("content") : "";
}
function apiGet(path) {
    return $.ajax({ url: "." + path, type: "GET", dataType: "json" });
}
function apiPostJSON(path, obj) {
    return $.ajax({
        url: "." + path, type: "POST",
        contentType: "application/json",
        headers: { "X-CSRF-Token": csrf() },
        data: JSON.stringify(obj || {}),
        dataType: "json"
    });
}
function apiPost(path) {
    return $.ajax({ url: "." + path, type: "POST", headers: { "X-CSRF-Token": csrf() }, dataType: "json" });
}

// ---------- Theme (driven by Zoraxy via shared localStorage) ----------
function setDarkTheme(isDark) {
    document.body.classList.toggle("darkTheme", !!isDark);
    document.documentElement.classList.toggle("darkTheme", !!isDark);
    localStorage.setItem("theme", isDark ? "dark" : "light");
}
if (localStorage.getItem("theme") === "dark") {
    document.documentElement.classList.add("darkTheme");
    document.body.classList.add("darkTheme");
}

// ---------- toast ----------
let toastTimer = null;
function toast(msg, ok) {
    const t = $("#toast");
    t.text(msg).addClass("show");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.removeClass("show"); }, 3000);
}
function escapeHtml(s) { return $("<div>").text(s == null ? "" : s).html(); }
function errMsg(xhr) { try { return (xhr.responseJSON && xhr.responseJSON.error) || xhr.statusText; } catch (e) { return "error"; } }

// ---------- confirm modal ----------
let _confirmCb = null;
function confirmBox(msg, cb) {
    $("#confirmMsg").text(msg);
    _confirmCb = cb;
    openModal("confirmModal");
}
function _confirmFinish(choice) {
    closeModal("confirmModal");
    const cb = _confirmCb; _confirmCb = null;
    if (cb) cb(choice);
}

// ---------- modal helpers ----------
function openModal(id) { $("#" + id).addClass("show"); }
function closeModal(id) { $("#" + id).removeClass("show"); }

// ---------- view switching ----------
function switchView(view) {
    currentView = view;
    $(".nav-link").removeClass("active");
    $('.nav-link[data-view="' + view + '"]').addClass("active");
    $(".view").removeClass("active");
    $('.view[data-view="' + view + '"]').addClass("active");
    if (view === "captures") loadCaptures();
    if (view === "rules") loadRules();
    if (view === "settings") loadSettings();
}

// ---------- Live captures ----------
function timeAgo(ms) {
    const d = Date.now() - ms;
    if (d < 1000) return "just now";
    const s = Math.floor(d / 1000);
    if (s < 60) return s + "s ago";
    const m = Math.floor(s / 60);
    if (m < 60) return m + "m ago";
    const h = Math.floor(m / 60);
    if (h < 24) return h + "h ago";
    return Math.floor(h / 24) + "d ago";
}

function loadCaptures() {
    apiGet("/api/captures").done(function (list) {
        capturesCache = list || [];
        renderCaptureList();
        // keep the detail pane in sync with the selected item
        if (selectedId && capturesCache.some(c => c.id === selectedId)) {
            renderDetail(capturesCache.find(c => c.id === selectedId));
        } else if (!selectedId && capturesCache.length === 0) {
            // leave empty state
        }
    }).fail(function (xhr) { toast(errMsg(xhr), false); });
}

function renderCaptureList() {
    const box = $("#capList");
    if (!capturesCache.length) {
        box.html('<div class="empty" style="padding:2rem 0;">No captures yet.</div>');
        return;
    }
    let html = "";
    capturesCache.forEach(function (c) {
        const target = escapeHtml(c.host + c.path);
        const modeBadge = c.mode === "tap"
            ? '<span class="badge badge-mode-tap">tap</span>'
            : '<span class="badge badge-mode-intercept">intercept</span>';
        html += '<div class="cap-item ' + (c.id === selectedId ? "active" : "") + '" onclick="selectCapture(\'' + c.id + '\')">'
            + '<div class="cap-item-top">'
            + '<span class="badge badge-method">' + escapeHtml(c.method) + '</span>'
            + '<span class="cap-target">' + target + '</span>'
            + '</div>'
            + '<div class="cap-item-top">'
            + modeBadge
            + '<span class="cap-time">' + timeAgo(c.time) + '</span>'
            + '</div>'
            + '</div>';
    });
    box.html(html);
}

function selectCapture(id) {
    selectedId = id;
    renderCaptureList();
    const c = capturesCache.find(x => x.id === id);
    if (c) renderDetail(c);
}

function kvRows(obj, empty) {
    const keys = Object.keys(obj || {}).sort();
    if (!keys.length) return '<tr><td colspan="2" class="empty">' + empty + '</td></tr>';
    let html = "";
    keys.forEach(function (k) {
        const vals = obj[k];
        if (Array.isArray(vals)) {
            vals.forEach(function (v) {
                html += '<tr><td class="k">' + escapeHtml(k) + '</td><td class="v">' + escapeHtml(v) + '</td></tr>';
            });
        } else {
            html += '<tr><td class="k">' + escapeHtml(k) + '</td><td class="v">' + escapeHtml(vals) + '</td></tr>';
        }
    });
    return html;
}

function renderDetail(c) {
    const dt = new Date(c.time);
    const modeBadge = c.mode === "tap"
        ? '<span class="badge badge-mode-tap">tap</span>'
        : '<span class="badge badge-mode-intercept">intercept</span>';

    let bodySection = "";
    if (c.mode === "tap" && !c.body_captured) {
        bodySection = '<div class="note" style="margin:0;">Body is not captured in tap mode (metadata only).</div>';
    } else if (!c.body_preview) {
        bodySection = '<div class="note" style="margin:0;">Empty body' + (c.body_size >= 0 ? " (" + c.body_size + " bytes)" : "") + '.</div>';
    } else {
        const note = c.body_truncated ? "truncated preview" : (c.body_size + " bytes");
        bodySection = '<div class="hint" style="margin:0 0 0.4rem;">' + note + '</div><pre class="body">' + escapeHtml(c.body_preview) + '</pre>';
    }

    const html =
        '<div class="panel panel-pad">'
        + '<div class="detail-head">'
        + '<div>'
        + '<div class="detail-title"><span class="badge badge-method">' + escapeHtml(c.method) + '</span> ' + escapeHtml(c.host + c.path) + '</div>'
        + '<div class="detail-sub">' + modeBadge + ' &middot; rule <b>' + escapeHtml(c.rule_name) + '</b> &middot; ' + escapeHtml(dt.toLocaleString()) + ' &middot; ' + escapeHtml(c.proto) + '</div>'
        + '</div>'
        + '<div class="detail-actions"><button class="btn btn-ghost btn-sm" onclick="copyDetail()"><span class="icon icon-copy"></span> Copy</button></div>'
        + '</div>'

        + '<div class="sec"><div class="sec-title">Request</div><table class="kv">'
        + '<tr><td class="k">Method</td><td class="v">' + escapeHtml(c.method) + '</td></tr>'
        + '<tr><td class="k">Host</td><td class="v">' + escapeHtml(c.host) + '</td></tr>'
        + '<tr><td class="k">Request-URI</td><td class="v">' + escapeHtml(c.request_uri) + '</td></tr>'
        + '<tr><td class="k">Remote Address</td><td class="v">' + escapeHtml(c.remote_addr) + '</td></tr>'
        + '<tr><td class="k">Content-Length</td><td class="v">' + escapeHtml(c.content_length) + '</td></tr>'
        + '</table></div>'

        + '<div class="sec"><div class="sec-title">Headers</div><table class="kv">' + kvRows(c.headers, "No headers") + '</table></div>'
        + '<div class="sec"><div class="sec-title">Cookies</div><table class="kv">' + kvRows(c.cookies, "No cookies") + '</table></div>'
        + '<div class="sec"><div class="sec-title">Query Parameters</div><table class="kv">' + kvRows(c.query, "No query parameters") + '</table></div>'
        + '<div class="sec"><div class="sec-title">Request Body</div>' + bodySection + '</div>'
        + '</div>';

    $("#mainPane").html(html);
}

function copyDetail() {
    const c = capturesCache.find(x => x.id === selectedId);
    if (!c) return;
    navigator.clipboard.writeText(JSON.stringify(c, null, 2)).then(
        function () { toast("Capture copied to clipboard"); },
        function () { toast("Copy failed", false); }
    );
}

function clearCaptures() {
    confirmBox("Clear the entire capture log?", function (ok) {
        if (!ok) return;
        apiPost("/api/captures/clear").done(function () {
            selectedId = null;
            $("#mainPane").html('<div class="panel panel-pad"><div class="empty"><div class="big">🔍</div>Select a captured request to inspect it, or send a request that matches one of your rules.</div></div>');
            loadCaptures();
            toast("Capture log cleared");
        }).fail(function (xhr) { toast(errMsg(xhr), false); });
    });
}

function toggleAuto() {
    if ($("#autoRefresh").is(":checked")) startAuto(); else stopAuto();
}
function startAuto() {
    stopAuto();
    refreshTimer = setInterval(function () {
        if (currentView === "captures") loadCaptures();
    }, 3000);
}
function stopAuto() { if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; } }

// ---------- Capture rules ----------
function loadRules() {
    apiGet("/api/rules").done(function (rules) {
        renderRules(rules || []);
    }).fail(function (xhr) { toast(errMsg(xhr), false); });
}

function renderRules(rules) {
    const tb = $("#ruleList");
    if (!rules.length) {
        tb.html('<tr><td colspan="5" class="empty">No rules yet. Add one to start capturing.</td></tr>');
        return;
    }
    let html = "";
    rules.forEach(function (r) {
        const modeBadge = r.mode === "tap"
            ? '<span class="badge badge-mode-tap">tap</span>'
            : '<span class="badge badge-mode-intercept">intercept</span>';
        html += '<tr>'
            + '<td>' + escapeHtml(r.name) + '</td>'
            + '<td><code>' + escapeHtml(r.pattern) + '</code></td>'
            + '<td>' + modeBadge + '</td>'
            + '<td><label class="switch"><input type="checkbox" ' + (r.enabled ? "checked" : "") + ' onchange="toggleRule(\'' + r.id + '\')"><span class="track"></span></label></td>'
            + '<td><div class="row-actions">'
            + '<button class="btn btn-ghost btn-sm" onclick=\'editRule(' + JSON.stringify(JSON.stringify(r)) + ')\'><span class="icon icon-edit"></span></button>'
            + '<button class="btn btn-ghost btn-sm" onclick="deleteRule(\'' + r.id + '\',\'' + escapeHtml(r.name).replace(/'/g, "") + '\')"><span class="icon icon-delete"></span></button>'
            + '</div></td>'
            + '</tr>';
    });
    tb.html(html);
}

function onModeChange() {
    $("#prettyField").css("visibility", $("#rMode").val() === "intercept" ? "visible" : "hidden");
}

function openRuleModal() {
    $("#ruleModalTitle").text("Add Rule");
    $("#rId").val("");
    $("#rName").val("");
    $("#rPattern").val("");
    $("#rMode").val("intercept");
    $("#rPretty").val("true");
    $("#rEnabled").prop("checked", true);
    onModeChange();
    openModal("ruleModal");
}

function editRule(json) {
    const r = typeof json === "string" ? JSON.parse(json) : json;
    $("#ruleModalTitle").text("Edit Rule");
    $("#rId").val(r.id);
    $("#rName").val(r.name);
    $("#rPattern").val(r.pattern);
    $("#rMode").val(r.mode);
    $("#rPretty").val(r.pretty_print ? "true" : "false");
    $("#rEnabled").prop("checked", !!r.enabled);
    onModeChange();
    openModal("ruleModal");
}

function saveRule() {
    const pattern = $("#rPattern").val().trim();
    if (!pattern) { toast("Pattern is required", false); return; }
    const payload = {
        id: $("#rId").val(),
        name: $("#rName").val().trim(),
        pattern: pattern,
        mode: $("#rMode").val(),
        pretty_print: $("#rPretty").val() === "true",
        enabled: $("#rEnabled").is(":checked")
    };
    apiPostJSON("/api/rule/save", payload).done(function () {
        closeModal("ruleModal");
        loadRules();
        toast("Rule saved");
    }).fail(function (xhr) { toast(errMsg(xhr), false); });
}

function toggleRule(id) {
    apiPost("/api/rule/toggle?id=" + encodeURIComponent(id)).done(function () {
        // no toast — the switch itself is the feedback
    }).fail(function (xhr) { toast(errMsg(xhr), false); loadRules(); });
}

function deleteRule(id, name) {
    confirmBox("Delete rule \"" + name + "\"?", function (ok) {
        if (!ok) return;
        apiPost("/api/rule/delete?id=" + encodeURIComponent(id)).done(function () {
            loadRules();
            toast("Rule deleted");
        }).fail(function (xhr) { toast(errMsg(xhr), false); });
    });
}

// ---------- Settings ----------
function loadSettings() {
    apiGet("/api/settings/get").done(function (s) {
        $("#setLogLimit").val(s.log_limit);
        $("#setBodyPreview").val(s.body_preview_size);
    }).fail(function (xhr) { toast(errMsg(xhr), false); });
}

function saveSettings() {
    const logLimit = parseInt($("#setLogLimit").val(), 10) || 200;
    const bodyPreview = parseInt($("#setBodyPreview").val(), 10);
    apiPost("/api/settings/save?log_limit=" + logLimit + "&body_preview_size=" + (isNaN(bodyPreview) ? 4096 : bodyPreview))
        .done(function () { toast("Settings saved"); loadSettings(); })
        .fail(function (xhr) { toast(errMsg(xhr), false); });
}

// ---------- init ----------
$(function () {
    setDarkTheme(localStorage.getItem("theme") === "dark");
    loadCaptures();
    startAuto();
    $(".modal-overlay").on("mousedown", function (e) {
        if (e.target !== this) return;
        if (this.id === "confirmModal") _confirmFinish(false);
        else closeModal(this.id);
    });
});
