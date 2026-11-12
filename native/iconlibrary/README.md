# Icon Library

This package exposes a UI-independent catalog of Lucide SVG icons for any Muse consumer. It contains icon identifiers, readable names, searchable tags, and the original SVG source; it does not depend on MyGo, nativeui, or the navigation-icon cache.

Catalog data is generated from the official [`lucide-static`](https://github.com/lucide-icons/lucide/tree/main/packages/lucide-static) package, version `1.72.0`. The included Lucide icons are under the ISC license; see [LICENSE](LICENSE).

To refresh the bundled catalog, extract `icons/`, `tags.json`, and `LICENSE` from the desired `lucide-static` package version. Generate `catalog.json.gz` with metadata entries (`id`, `name`, `tags`) and `assets/lucide-icons.zip` with the individual SVG files. Keeping the SVG pack separate lets catalog search avoid reading icon vectors and lets consumers load a single SVG through `Lookup`.
