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
    function runFormHook(form, attr) {
        if (!form || !form.hasAttribute(attr)) {
            return;
        }
        var fn = window[form.getAttribute(attr)];
        if (typeof fn === "function") {
            fn();
        }
        if (form.hasAttribute("data-reset-form")) {
            form.reset();
        }
    }

    document.body.addEventListener("htmx:afterRequest", function (e) {
        var form = e.target && e.target.closest ? e.target.closest("form[data-after]") : null;
        runFormHook(form, "data-after");
    });

    document.body.addEventListener("htmx:afterSwap", function (e) {
        var form = e.target && e.target.closest ? e.target.closest("form[data-after-swap]") : null;
        runFormHook(form, "data-after-swap");
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

    // Menu builder link picker (content arrives via HTMX, hence delegation).
    document.addEventListener("change", function (e) {
        if (e.target && e.target.id === "link_source") {
            var source = e.target;
            var url = document.getElementById("menu_item_link");
            var title = document.getElementById("menu_item_title");
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
