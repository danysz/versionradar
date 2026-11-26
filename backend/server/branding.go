package server

import (
	"bytes"
	_ "embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/ui"
)

//go:embed logo.svg
var BrandLogoSVG []byte

//go:embed favicon.png
var BrandFaviconPNG []byte

const brandingExtensionJS = `
if (window.app && window.app.store) {
	window.app.store.favicon = "./images/favicon.png";
}

document.addEventListener("mount:pageInstaller", () => {
	if (window.app && window.app.store) {
		window.app.store.title = "Setup your VersionRadar instance";
	}
});

const applyBranding = () => {
	if (document.title === "PocketBase") {
		document.title = "VersionRadar";
	} else if (document.title.includes("PocketBase")) {
		document.title = document.title.replaceAll("PocketBase", "VersionRadar");
	}

	if (!document.body) {
		return;
	}
	const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
	let node;
	while ((node = walker.nextNode())) {
		if (node.nodeValue && node.nodeValue.includes("Setup your PocketBase instance")) {
			node.nodeValue = node.nodeValue.replaceAll(
				"Setup your PocketBase instance",
				"Setup your VersionRadar instance",
			);
		}
	}
};

applyBranding();

if (typeof MutationObserver !== "undefined") {
	const target = document.documentElement || document.body;
	if (target) {
		const observer = new MutationObserver(() => {
			applyBranding();
		});
		observer.observe(target, {
			childList: true,
			subtree: true,
			characterData: true,
		});
	}
}
`

func init() {
	wrapDistDirFS()
}

func wrapDistDirFS() {
	if ui.DistDirFS == nil {
		return
	}
	if _, ok := ui.DistDirFS.(*brandedUIFS); ok {
		return
	}
	ui.DistDirFS = newBrandedUIFS(ui.DistDirFS)
}

type brandedUIFS struct {
	base fs.FS
}

func newBrandedUIFS(base fs.FS) *brandedUIFS {
	return &brandedUIFS{base: base}
}

func shouldRewriteUIFile(name string) bool {
	return name == "index.html" || strings.HasSuffix(name, ".js")
}

func rewriteUIBytes(name string, raw []byte) []byte {
	if name == "index.html" {
		s := string(raw)
		s = strings.ReplaceAll(s, "<title>PocketBase</title>", "<title>VersionRadar</title>")
		s = strings.ReplaceAll(s, "Setup your PocketBase instance", "Setup your VersionRadar instance")
		if !strings.Contains(s, `href="./images/favicon.png"`) {
			const iconTag = `<link rel="icon" type="image/png" href="./images/favicon.png">`
			if strings.Contains(s, "</head>") {
				s = strings.Replace(s, "</head>", "  "+iconTag+"\n</head>", 1)
			} else {
				s = iconTag + "\n" + s
			}
		}
		return []byte(s)
	}

	if bytes.Contains(raw, []byte("Setup your PocketBase instance")) {
		return bytes.ReplaceAll(raw, []byte("Setup your PocketBase instance"), []byte("Setup your VersionRadar instance"))
	}
	return raw
}

func (b *brandedUIFS) Open(name string) (fs.File, error) {
	f, err := b.base.Open(name)
	if err != nil {
		return nil, err
	}

	if !shouldRewriteUIFile(name) {
		return f, nil
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if fi.IsDir() {
		return f, nil
	}

	raw, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}

	modified := rewriteUIBytes(name, raw)
	return &brandedFile{
		Reader: bytes.NewReader(modified),
		info:   &brandedFileInfo{FileInfo: fi, size: int64(len(modified))},
	}, nil
}

func (b *brandedUIFS) Stat(name string) (fs.FileInfo, error) {
	if shouldRewriteUIFile(name) {
		f, err := b.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.Stat()
	}
	return fs.Stat(b.base, name)
}

func (b *brandedUIFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(b.base, name)
}

type brandedFile struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *brandedFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

func (f *brandedFile) Close() error {
	return nil
}

type brandedFileInfo struct {
	fs.FileInfo
	size int64
}

func (fi *brandedFileInfo) Size() int64 {
	return fi.size
}

type staticFileFS struct {
	name string
	data []byte
}

func (s staticFileFS) Open(name string) (fs.File, error) {
	if name != s.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &brandedFile{
		Reader: bytes.NewReader(s.data),
		info:   staticFileInfo{name: s.name, size: int64(len(s.data))},
	}, nil
}

type staticFileInfo struct {
	name string
	size int64
}

func (fi staticFileInfo) Name() string       { return fi.name }
func (fi staticFileInfo) Size() int64        { return fi.size }
func (fi staticFileInfo) Mode() fs.FileMode  { return 0o444 }
func (fi staticFileInfo) ModTime() time.Time { return time.Time{} }
func (fi staticFileInfo) IsDir() bool        { return false }
func (fi staticFileInfo) Sys() any           { return nil }

func registerBranding(app core.App) {
	wrapDistDirFS()

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		settings := app.Settings()
		if settings.Meta.AppName == "" || settings.Meta.AppName == "Acme" || settings.Meta.AppName == "PocketBase" {
			settings.Meta.AppName = "VersionRadar"
			settings.Meta.SenderName = "VersionRadar"
			_ = app.Save(settings)
		}
		return nil
	})

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/_/images/favicon.png", func(re *core.RequestEvent) error {
			return re.Blob(http.StatusOK, "image/png", BrandFaviconPNG)
		})
		e.Router.GET("/_/images/favicon_prod.png", func(re *core.RequestEvent) error {
			return re.Blob(http.StatusOK, "image/png", BrandFaviconPNG)
		})
		e.Router.GET("/_/images/logo_white.svg", func(re *core.RequestEvent) error {
			return re.Blob(http.StatusOK, "image/svg+xml", BrandLogoSVG)
		})
		e.Router.GET("/_/images/logo.svg", func(re *core.RequestEvent) error {
			return re.Blob(http.StatusOK, "image/svg+xml", BrandLogoSVG)
		})
		e.Router.GET("/favicon.ico", func(re *core.RequestEvent) error {
			return re.Blob(http.StatusOK, "image/svg+xml", BrandLogoSVG)
		})

		e.UIExtensions = append(e.UIExtensions, core.UIExtension{
			Name: "versionradar-branding",
			FS: staticFileFS{
				name: "main.js",
				data: []byte(brandingExtensionJS),
			},
		})

		return e.Next()
	})
}
