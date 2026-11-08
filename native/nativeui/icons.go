package nativeui

import "github.com/egoist/mygo/ui"

// The icons are Lucide's drawings, on its 24-unit grid, stroked in whatever
// colour the element they sit in carries. One definition serves the pure-white
// shell and the near-black one.
func lucide(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ` +
			`stroke="currentColor" stroke-width="2.4" stroke-linecap="round" ` +
			`stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconNew = lucide(`<path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>` +
		`<path d="M18.4 2.6a2.1 2.1 0 1 1 3 3L12 15l-4 1 1-4Z"/>`)
	iconOpenFile = lucide(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>` +
		`<path d="M14 2v6h6"/><path d="M12 18v-6"/><path d="m9 15 3-3 3 3"/>`)
	iconFolderOpen = lucide(`<path d="m6 14 1.45-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.55 6a2 2 0 0 1-1.94 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.93a2 2 0 0 1 1.66.9l.82 1.2a2 2 0 0 0 1.66.9H18a2 2 0 0 1 2 2v2"/>`)
	iconFiles      = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>` +
		`<path d="M8 10h8"/><path d="M8 14h6"/>`)
	iconOutline = lucide(`<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/>` +
		`<path d="M3 6h.01"/><path d="M3 12h.01"/><path d="M3 18h.01"/>`)
	iconSearch = lucide(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)
	iconAI     = lucide(`<path d="m12 3-1.9 5.8a2 2 0 0 1-1.3 1.3L3 12l5.8 1.9a2 2 0 0 1 1.3 1.3L12 21l1.9-5.8a2 2 0 0 1 1.3-1.3L21 12l-5.8-1.9a2 2 0 0 1-1.3-1.3Z"/>` +
		`<path d="M5 3v4"/><path d="M3 5h4"/><path d="M19 17v4"/><path d="M17 19h4"/>`)
	iconSun = lucide(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/>` +
		`<path d="M12 20v2"/><path d="m4.9 4.9 1.4 1.4"/><path d="m17.7 17.7 1.4 1.4"/>` +
		`<path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.3 17.7-1.4 1.4"/>` +
		`<path d="m19.1 4.9-1.4 1.4"/>`)
	iconMoon     = lucide(`<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>`)
	iconSettings = lucide(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2Z"/>` +
		`<circle cx="12" cy="12" r="3"/>`)
	iconFile = lucide(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>` +
		`<path d="M14 2v6h6"/>`)
	iconFolder   = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
	iconFilePlus = lucide(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>` +
		`<path d="M14 2v6h6"/><path d="M12 18v-6"/><path d="M9 15h6"/>`)
	iconFolderPlus = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>` +
		`<path d="M12 10v6"/><path d="M9 13h6"/>`)
	iconEdit = lucide(`<path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/><path d="m15 5 4 4"/>`)
	iconEye  = lucide(`<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>`)
	iconSave = lucide(`<path d="M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z"/>` +
		`<path d="M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7"/><path d="M7 3v4a1 1 0 0 0 1 1h7"/>`)
	iconClose          = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconWindowMinimize = lucide(`<path d="M5 12h14"/>`)
	iconWindowMaximize = lucide(`<rect width="14" height="14" x="5" y="5" rx="2"/>`)
	iconWindowRestore  = lucide(`<rect width="10" height="10" x="9" y="9" rx="1.5"/>` +
		`<path d="M6 15H5.5A1.5 1.5 0 0 1 4 13.5v-8A1.5 1.5 0 0 1 5.5 4h8A1.5 1.5 0 0 1 15 5.5V6"/>`)
	iconChevron        = lucide(`<path d="m9 18 6-6-6-6"/>`)
	iconChevronLeft    = lucide(`<path d="m15 18-6-6 6-6"/>`)
	iconInfo           = lucide(`<circle cx="12" cy="12" r="9"/><path d="M12 16v-4"/><path d="M12 8h.01"/>`)
	iconMore           = lucide(`<circle cx="5" cy="12" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="19" cy="12" r="1.6"/>`)
	iconChevronDown    = lucide(`<path d="m6 9 6 6 6-6"/>`)
	iconPanelLeftClose = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/>` +
		`<path d="M9 3v18"/><path d="m16 15-3-3 3-3"/>`)
	iconPanelLeftOpen = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/>` +
		`<path d="M9 3v18"/><path d="m14 9 3 3-3 3"/>`)
	iconArrowUp = lucide(`<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>`)
	iconStop    = lucide(`<rect width="10" height="10" x="7" y="7" rx="1.5" fill="currentColor" stroke="none"/>`)
	iconTrash   = lucide(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>` +
		`<path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
)
