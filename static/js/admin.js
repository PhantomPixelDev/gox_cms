// Admin helpers shared by every admin fragment. Loaded once from the main
// layout, so fragments must not redefine these (they used to, once per tab).
// Event delegation is used throughout: admin tables arrive later via HTMX.
(function () {
    "use strict";

    function refresh(selector, url) {
        htmx.ajax("GET", url, {
            target: selector,
            swap: "innerHTML",
            headers: { "X-No-Cache": "true" },
        });
    }

    window.updateFileList = function () {
        var progress = htmx.find("#progress");
        if (progress) {
            progress.setAttribute("value", 0);
        }
        var form = htmx.find("#upload-file-form");
        if (form) {
            form.reset();
        }
        refresh("#file-list-container", "/search-files");
    };

    window.updateSettingsForm = function () {
        refresh("#settings-container", "/admin-settings");
    };

    window.searchTags = function () {
        refresh("#tag-table-container", "/search-tags");
    };

    window.searchCategories = function () {
        refresh("#category-table-container", "/search-categories");
    };

    window.updateMenuList = function () {
        refresh("#menu-table-container", "/search-menu");
    };

    // Declarative post-request hooks without inline hx-on attributes (kept
    // CSP-clean): <form data-after="updateMenuList" data-reset-form>.
    // The trigger may be the element itself or an ancestor (buttons carrying
    // data-after used to be ignored, so the file list never refreshed).
    function runHook(el, attr) {
        if (!el) {
            return;
        }
        var owner = el.matches && el.matches("[" + attr + "]")
            ? el
            : el.closest("[" + attr + "]");
        if (!owner) {
            return;
        }
        var fn = window[owner.getAttribute(attr)];
        if (typeof fn === "function") {
            fn();
        }
        if (owner.hasAttribute("data-reset-form") && typeof owner.reset === "function") {
            owner.reset();
        }
    }

    document.body.addEventListener("htmx:afterRequest", function (e) {
        runHook(e.target, "data-after");
    });

    document.body.addEventListener("htmx:afterSwap", function (e) {
        runHook(e.target, "data-after-swap");
    });

    function showCopiedToast() {
        htmx.trigger(document.body, "showToast", { value: "URL Copied to Clipboard" });
    }

    window.copyToClipboard = function (text) {
        function legacyCopy(value) {
            var el = document.createElement("textarea");
            el.value = value;
            document.body.appendChild(el);
            el.select();
            document.execCommand("copy");
            document.body.removeChild(el);
        }
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).then(showCopiedToast, function () {
                legacyCopy(text);
                showCopiedToast();
            });
            return;
        }
        legacyCopy(text);
        showCopiedToast();
    };

    // Menu builder link picker: the "Link to" select fills the URL field and
    // suggests a title (content arrives via HTMX, hence delegation). IDs are
    // namespaced to the add form so the edit modal's inputs are never touched.
    document.addEventListener("change", function (e) {
        if (e.target && e.target.id === "new-item-source") {
            var source = e.target;
            var url = document.getElementById("new-item-link");
            var title = document.getElementById("new-item-title");
            if (!url) {
                return;
            }
            var opt = source.options[source.selectedIndex];
            if (source.value === "__custom") {
                url.value = "";
                url.focus();
                return;
            }
            url.value = source.value;
            if (title && !title.value && opt.getAttribute("data-title")) {
                title.value = opt.getAttribute("data-title");
            }
        }
    });

    // Bulk actions: gather checked rows, confirm deletes, POST, refresh.
    document.addEventListener("click", function (e) {
        var btn = e.target && e.target.closest ? e.target.closest("[data-bulk-endpoint]") : null;
        if (!btn) {
            return;
        }
        var scope = document.querySelector(btn.getAttribute("data-bulk-scope"));
        if (!scope) {
            return;
        }
        // The toolbar div holds both the button and its action select.
        var toolbar = btn.closest("div");
        var actionSel = toolbar ? toolbar.querySelector("[data-bulk-action]") : null;
        var action = actionSel ? actionSel.value : "";
        var ids = [];
        scope.querySelectorAll(".bulk-check:checked").forEach(function (box) {
            ids.push(box.value);
        });
        if (!action || ids.length === 0) {
            htmx.trigger(document.body, "showToast", { value: "Pick an action and select rows first" });
            return;
        }
        if (action === "delete" && !window.confirm("Apply '" + action + "' to " + ids.length + " item(s)?")) {
            return;
        }
        var params = new URLSearchParams();
        params.append("action", action);
        params.append("ids", ids.join(","));
        var match = document.cookie.match(/(?:^|;\s*)csrf_=([^;]*)/);
        var headers = { "Content-Type": "application/x-www-form-urlencoded" };
        if (match) {
            headers["X-Csrf-Token"] = decodeURIComponent(match[1]);
        }
        fetch(btn.getAttribute("data-bulk-endpoint"), {
            method: "POST",
            headers: headers,
            body: params.toString(),
        }).then(function (resp) {
            return resp.json().then(function (data) {
                return { ok: resp.ok, data: data };
            });
        }).then(function (result) {
            if (result.ok) {
                refresh(
                    btn.getAttribute("data-bulk-target"),
                    btn.getAttribute("data-bulk-refresh")
                );
            }
            htmx.trigger(document.body, "showToast", {
                value: result.ok
                    ? "Bulk action done"
                    : (result.data && result.data.error) || "Bulk action failed",
            });
        }).catch(function () {
            htmx.trigger(document.body, "showToast", { value: "Bulk action failed" });
        });
    });

    // Select-all checkboxes (scoped to their table).
    document.addEventListener("change", function (e) {
        var master = e.target && e.target.closest ? e.target.closest("[data-select-all]") : null;
        if (!master) {
            return;
        }
        var table = master.closest("table");
        if (!table) {
            return;
        }
        table.querySelectorAll(".bulk-check").forEach(function (box) {
            box.checked = master.checked;
        });
    });

    // Copy buttons rendered by the file manager (no inline onclick).
    document.addEventListener("click", function (e) {
        var btn = e.target && e.target.closest ? e.target.closest("[data-copy-url]") : null;
        if (btn) {
            window.copyToClipboard(btn.getAttribute("data-copy-url"));
        }
    });

    // Upload progress bar (file manager tab).
    document.addEventListener("DOMContentLoaded", function () {
        htmx.on("#upload-file-form", "htmx:xhr:progress", function (evt) {
            var bar = htmx.find("#progress");
            if (bar && evt.detail.total > 0) {
                bar.setAttribute("value", (evt.detail.loaded / evt.detail.total) * 100);
            }
        });
    });
})();
