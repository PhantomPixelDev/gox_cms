// Rich-text editor wiring for post/page add/edit screens. Every block is
// guarded: this file loads on all pages but only acts when its elements and
// libraries (Quill, jQuery/selectize) are present.
(function () {
    "use strict";

    function initQuill() {
        var editorEl = document.getElementById("editor");
        if (!editorEl || typeof Quill === "undefined") {
            return;
        }
        var quill = new Quill("#editor", {
            modules: {
                toolbar: [
                    [{ header: "1" }, { header: "2" }, { header: [3, 4, 5, 6] }, { font: [] }],
                    [{ size: [] }],
                    ["bold", "italic", "underline", "strike", "blockquote"],
                    [{ list: "ordered" }, { list: "bullet" }, { indent: "-1" }, { indent: "+1" }],
                    ["link", "image", "video"],
                    ["clean"],
                    ["code-block"],
                    [{ color: [] }, { background: [] }],
                    [{ align: [] }],
                ],
                theme: "snow",
            },
        });

        // Initial content comes from data-content (Go-escaped); the DOM
        // gives us the decoded HTML back.
        var initial = editorEl.getAttribute("data-content");
        if (initial) {
            quill.root.innerHTML = initial;
        }

        // Exposed for the file-picker Insert buttons below.
        window._goxQuill = quill;

        function sync() {
            var contentInput = document.getElementById("content_input");
            if (contentInput) {
                contentInput.value = quill.root.innerHTML;
            }
        }
        quill.on("text-change", sync);

        var form = editorEl.closest("form");
        if (form) {
            form.addEventListener("submit", sync);
        }
    }

    function initSelectize() {
        if (typeof window.jQuery === "undefined") {
            return;
        }
        [["categoriesSelect", "categories_input", "Select categories"],
            ["tagsSelect", "tags_input", "Select tags"],
        ].forEach(function (pair) {
            var select = document.getElementById(pair[0]);
            var hidden = document.getElementById(pair[1]);
            if (!select || !hidden || typeof window.jQuery(select).selectize !== "function") {
                return;
            }
            window.jQuery(select).selectize({
                plugins: ["remove_button"],
                placeholder: pair[2],
                // In-place creation: type a new name, Enter sends it to the
                // server and selects the returned ID.
                create: function (input, callback) {
                    var params = new URLSearchParams();
                    params.append("kind", select.getAttribute("data-kind"));
                    params.append("name", input);
                    var match = document.cookie.match(/(?:^|;\s*)csrf_=([^;]*)/);
                    var headers = { "Content-Type": "application/x-www-form-urlencoded" };
                    if (match) {
                        headers["X-Csrf-Token"] = decodeURIComponent(match[1]);
                    }
                    fetch("/add-taxonomy", {
                        method: "POST",
                        headers: headers,
                        body: params.toString(),
                    }).then(function (resp) {
                        if (!resp.ok) {
                            throw new Error("create failed");
                        }
                        return resp.json();
                    }).then(function (data) {
                        callback({ value: data.id, text: input });
                    }).catch(function () {
                        callback();
                    });
                },
                sortField: "text",
                onChange: function (value) {
                    hidden.value = value.join(",");
                },
            });
            // Pre-select values rendered server-side (edit screens).
            var pre = select.getAttribute("data-selected");
            if (pre && select.selectize) {
                select.selectize.setValue(pre.split(","));
                hidden.value = pre;
            }
        });

        document.body.addEventListener("clearForm", function () {
            // Scoped to the form that owns the editor. This used
            // document.querySelector("form"), which picks whichever form is
            // first in the document — on a page carrying the file-picker
            // that is only correct by accident of DOM order.
            var editorEl = document.getElementById("editor");
            var form = editorEl ? editorEl.closest("form") : null;
            if (form) {
                form.reset();
                ["categoriesSelect", "tagsSelect"].forEach(function (id) {
                    var el = form.querySelector("#" + id);
                    if (el && el.selectize) {
                        el.selectize.clear();
                    }
                });
            }
            var quillEl = document.querySelector(".ql-editor");
            if (quillEl) {
                quillEl.innerHTML = "";
            }
            markClean();
        });
    }

    function initSlugAndPreview() {
        // Auto-slug from the title until the user edits the slug by hand.
        // Post forms use #post_slug, custom-page forms use #slug.
        var title = document.getElementById("title");
        var slug = document.getElementById("post_slug") || document.getElementById("slug");
        if (title && slug) {
            var touched = false;
            slug.addEventListener("input", function () {
                touched = true;
            });
            title.addEventListener("input", function () {
                if (touched) {
                    return;
                }
                slug.value = title.value
                    .toLowerCase()
                    .trim()
                    .replace(/[^a-z0-9]+/g, "-")
                    .replace(/^-+|-+$/g, "");
            });
        }

        // Live preview for the featured-image URL.
        var input = document.getElementById("image");
        var preview = document.getElementById("image-preview");
        if (input && preview) {
            input.addEventListener("input", function () {
                if (input.value.trim() === "") {
                    preview.classList.add("d-none");
                    preview.removeAttribute("src");
                    return;
                }
                preview.src = input.value.trim();
                preview.classList.remove("d-none");
            });
        }
    }

    // Insert an image at the cursor position from the file picker. Only
    // acts inside the picker modal (data-picker container); elsewhere the
    // button explains where to go.
    document.addEventListener("click", function (e) {
        var btn = e.target && e.target.closest ? e.target.closest("[data-insert-url]") : null;
        if (!btn) {
            return;
        }
        var url = btn.getAttribute("data-insert-url");
        var inPicker = btn.closest("[data-picker]");
        if (window._goxQuill && inPicker) {
            var range = window._goxQuill.getSelection(true);
            window._goxQuill.insertEmbed(range.index, "image", url);
            syncEditorInput();
            var modalEl = document.getElementById("exampleModal");
            if (modalEl && window.bootstrap) {
                var modal = window.bootstrap.Modal.getInstance(modalEl);
                if (modal) {
                    modal.hide();
                }
            }
        } else {
            htmx.trigger(document.body, "showToast", {
                value: "Open a post or page to insert images",
            });
        }
    });

    function syncEditorInput() {
        var contentInput = document.getElementById("content_input");
        if (contentInput && window._goxQuill) {
            contentInput.value = window._goxQuill.root.innerHTML;
        }
    }

    // Draft preview: POST the unsaved form through the real template and
    // open the rendered page in a new tab. Nothing is stored server-side.
    document.addEventListener("click", function (e) {
        var btn = e.target && e.target.closest ? e.target.closest("[data-preview]") : null;
        if (!btn) {
            return;
        }
        syncEditorInput();
        var titleEl = document.getElementById("title");
        var contentEl = document.getElementById("content_input");
        var imageEl = document.getElementById("image");
        var params = new URLSearchParams();
        params.append("title", titleEl ? titleEl.value : "");
        params.append("content", contentEl ? contentEl.value : "");
        if (imageEl) {
            params.append("image", imageEl.value);
        }
        var match = document.cookie.match(/(?:^|;\s*)csrf_=([^;]*)/);
        var headers = { "Content-Type": "application/x-www-form-urlencoded" };
        if (match) {
            headers["X-Csrf-Token"] = decodeURIComponent(match[1]);
        }
        // Open a blank tab synchronously: popup blockers only allow
        // window.open() inside the user-gesture task, so waiting on the
        // fetch first gets the window blocked.
        var tab = window.open("", "_blank");
        fetch("/preview-" + btn.getAttribute("data-preview"), {
            method: "POST",
            headers: headers,
            body: params.toString(),
        }).then(function (resp) {
            if (!resp.ok) {
                throw new Error("preview failed");
            }
            return resp.text();
        }).then(function (html) {
            if (!tab) {
                throw new Error("popup blocked");
            }
            // Writing into about:blank leaves the document with an opaque
            // origin, so every relative asset (/static/css/styles.css, post
            // images) resolved against about:blank and 404'd. A blob: URL
            // keeps them relative to the real site.
            var blobUrl = URL.createObjectURL(new Blob([html], { type: "text/html" }));
            tab.location.replace(blobUrl);
            // A preview is not an edit, so stop the unsaved-changes guard
            // from firing when the admin leaves the editor afterwards.
            markClean();
        }).catch(function () {
            if (tab) {
                tab.close();
            }
            htmx.trigger(document.body, "showToast", { value: "Preview needs a title" });
        });
    });

    // Unsaved-changes guard: navigating away with edits warns first.
    // Armed only on pages carrying an editor. The flag is module-level so the
    // preview handler can clear it.
    var formIsDirty = false;

    function markClean() {
        formIsDirty = false;
    }

    function initDirtyGuard() {
        if (!document.getElementById("editor")) {
            return;
        }
        document.addEventListener("input", function (e) {
            if (e.target && e.target.closest && e.target.closest("form")) {
                formIsDirty = true;
            }
        });
        document.addEventListener("submit", function () {
            markClean();
        });
        window.addEventListener("beforeunload", function (e) {
            if (formIsDirty) {
                e.preventDefault();
                e.returnValue = "";
            }
        });
        // Quill edits bypass input events; hook them once the editor exists.
        var quillTimer = setInterval(function () {
            if (window._goxQuill) {
                window._goxQuill.on("text-change", function () {
                    formIsDirty = true;
                });
                clearInterval(quillTimer);
            }
        }, 500);
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", function () {
            initQuill();
            initSelectize();
            initSlugAndPreview();
            initDirtyGuard();
        });
    } else {
        initQuill();
        initSelectize();
        initSlugAndPreview();
        initDirtyGuard();
    }
})();
