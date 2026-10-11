package screenshot

import "github.com/egoist/mygo/ui"

func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	iconRect    = icon(`<rect x="4" y="5" width="16" height="14" rx="1"/>`)
	iconArrow   = icon(`<path d="M6 18L18 6M9 6h9v9"/>`)
	iconPen     = icon(`<path d="M4 20l1-4L16 5l3 3L8 19z"/>`)
	iconText    = icon(`<path d="M5 7V5h14v2M12 5v14M9 19h6"/>`)
	iconUndo    = icon(`<path d="M9 14L4 9l5-5M4 9h9C16 9 19 11 19 14.5C19 18 16 20 13 20h-2"/>`)
	iconSave    = icon(`<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>`)
	iconClose   = icon(`<path d="M6 6l12 12M18 6L6 18"/>`)
	iconCheck   = icon(`<path d="M5 12l5 5 9-10"/>`)
	iconLibrary = icon(`<path d="M5 4h4v16H5zM10 4h4v16h-4zM15.5 5.2l3.4-.9 2.6 14.6-3.4.9z"/>`)
	iconClear   = icon(`<circle cx="12" cy="12" r="9" fill="currentColor" stroke="none"/><path d="M9 9l6 6M15 9l-6 6" stroke="#fff"/>`)
)
