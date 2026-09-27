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

        function show(el, body, value) {
            if (!el || !body) {
                return;
            }
            body.textContent = value;
            if (window.bootstrap) {
                return;
            }
            // The "simple" template set ships no Bootstrap, so drive the
            // .show class the framework-free stylesheet already styles.
            el.classList.add("show");
            window.clearTimeout(el._goxTimer);
            el._goxTimer = window.setTimeout(function () {
                el.classList.remove("show");
            }, 3000);
        }

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
        } else {
            htmx.on("showToast", function (e) {
                show(toastElement, toastBody, e.detail.value);
            });
            htmx.on("ShowToastError", function (e) {
                show(toastErrorElement, toastErrorBody, e.detail.value);
            });
        }

        // Bootstrap reads data-bs-theme from <html>, so toggle it there.
        var root = document.documentElement;

        // Apply the visitor's saved theme, if any. Without a saved choice the
        // server-rendered data-bs-theme stands (a dark Bootswatch theme starts
        // dark and must not be reset to light here).
        var theme = localStorage.getItem("data-bs-theme");
        if (theme === "dark" || theme === "light") {
            root.setAttribute("data-bs-theme", theme);
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
