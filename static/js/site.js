// Site-wide behavior: CSRF header wiring for HTMX, toasts, theme restore.
// Loaded with defer from the main layout; everything here is guarded so the
// file is safe on pages without the matching elements.
(function () {
    "use strict";

    // Send the CSRF token (kept in the csrf_ cookie) with every HTMX request.
    document.addEventListener("htmx:configRequest", function (event) {
        var match = document.cookie.match(/(?:^|;\s*)csrf_=([^;]*)/);
        if (match) {
            event.detail.headers["X-Csrf-Token"] = decodeURIComponent(match[1]);
        }
    });

    document.addEventListener("DOMContentLoaded", function () {
        var toastElement = document.getElementById("toast");
        var toastBody = document.getElementById("toast-body");
        var toastErrorElement = document.getElementById("toast-error");
        var toastErrorBody = document.getElementById("toast-body-error");

        if (window.bootstrap && toastElement && toastErrorElement) {
            var toast = new bootstrap.Toast(toastElement, { delay: 2000 });
            var toastError = new bootstrap.Toast(toastErrorElement, { delay: 2000 });

            htmx.on("showToast", function (e) {
                toastBody.textContent = e.detail.value;
                toast.show();
            });

            htmx.on("ShowToastError", function (e) {
                toastErrorBody.textContent = e.detail.value;
                toastError.show();
            });
        }

        // Bootstrap reads data-bs-theme from <html>, so toggle it there.
        var root = document.documentElement;

        var theme = localStorage.getItem("data-bs-theme");
        if (theme === "dark") {
            root.setAttribute("data-bs-theme", "dark");
        } else {
            root.setAttribute("data-bs-theme", "light");
        }

        // Any element with this id flips the theme (the account page has one
        // alongside the navbar switch).
        document.addEventListener("click", function (e) {
            if (!e.target || !e.target.closest) {
                return;
            }
            var btn = e.target.closest("[data-theme-toggle]");
            if (!btn) {
                return;
            }
            var next = root.getAttribute("data-bs-theme") === "dark" ? "light" : "dark";
            root.setAttribute("data-bs-theme", next);
            localStorage.setItem("data-bs-theme", next);
            syncSwitchIcon();
        });

        // Keep the navbar and account-page icons showing the right state.
        function syncSwitchIcon() {
            var dark = root.getAttribute("data-bs-theme") === "dark";
            document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
                var icon = btn.querySelector("i.bi");
                if (icon) {
                    icon.className = "bi " + (dark ? "bi-sun" : "bi-moon-stars");
                }
                btn.setAttribute("aria-pressed", dark ? "true" : "false");
            });
        }
        syncSwitchIcon();
    });
})();
