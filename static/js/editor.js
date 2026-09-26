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
                create: false,
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
            var form = document.querySelector("form");
            if (form) {
                form.reset();
            }
            var quillEl = document.querySelector(".ql-editor");
            if (quillEl) {
                quillEl.innerHTML = "";
            }
            ["categoriesSelect", "tagsSelect"].forEach(function (id) {
                var el = document.getElementById(id);
                if (el && el.selectize) {
                    el.selectize.clear();
                }
            });
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

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", function () {
            initQuill();
            initSelectize();
            initSlugAndPreview();
        });
    } else {
        initQuill();
        initSelectize();
        initSlugAndPreview();
    }
})();
