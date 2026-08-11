package main

var hugoCases = []BenchCase{
	// CONCEPTUAL
	{Query: "building the set of hugo sites from configuration", Category: "conceptual", Expected: "hugolib/site.go"},
	{Query: "rendering the site to its output files", Category: "conceptual", Expected: "hugolib/site_render.go"},
	{Query: "rendering a page template for a content page", Category: "conceptual", Expected: "hugolib/page__content.go"},
	{Query: "expanding shortcodes inside markdown content", Category: "conceptual", Expected: "hugolib/shortcode.go"},
	{Query: "discovering content files on disk", Category: "conceptual", Expected: "hugolib/content_factory.go"},
	{Query: "building the in-memory content tree of pages", Category: "conceptual", Expected: "hugolib/content_map_page.go"},
	{Query: "attaching output format metadata to a page", Category: "conceptual", Expected: "output/output_format.go"},
	{Query: "converting markdown content to html", Category: "conceptual", Expected: "markup/goldmark/convert.go"},
	{Query: "rendering markdown link and image hooks", Category: "conceptual", Expected: "markup/goldmark/render_hooks.go"},
	{Query: "building the table of contents for a page", Category: "conceptual", Expected: "markup/goldmark/toc.go"},
	{Query: "minifying css and js output assets", Category: "conceptual", Expected: "minifiers/minifiers.go"},
	{Query: "collecting html elements during publish", Category: "conceptual", Expected: "publisher/htmlElementsCollector.go"},
	{Query: "publishing rendered pages to disk", Category: "conceptual", Expected: "publisher/publisher.go"},
	{Query: "iterating over a collection of pages", Category: "conceptual", Expected: "tpl/collections/index.go"},
	{Query: "appending values to a slice in templates", Category: "conceptual", Expected: "tpl/collections/append.go"},
	{Query: "parsing command line flags for the hugo binary", Category: "conceptual", Expected: "commands/commands.go"},
	{Query: "handling the hugo serve command with hot reload", Category: "conceptual", Expected: "commands/hugobuilder.go"},
	{Query: "loading configuration from files into the runtime", Category: "conceptual", Expected: "config/config.go"},
	{Query: "caching transformed resources on disk", Category: "conceptual", Expected: "resources/resource_cache.go"},
	{Query: "processing an image resource through the image pipeline", Category: "conceptual", Expected: "resources/image.go"},
	{Query: "resolving the templates available for a site", Category: "conceptual", Expected: "tpl/templates/templates.go"},

	// EXACT SYMBOL
	{Query: "func NewHugoSites", Category: "exact_symbol", Expected: "hugolib/site.go"},
	{Query: "func (h *HugoSites) isRebuild", Category: "exact_symbol", Expected: "hugolib/hugo_sites.go"},
	{Query: "func (h *HugoSites) Build", Category: "exact_symbol", Expected: "hugolib/hugo_sites_build.go"},
	{Query: "type Page", Category: "exact_symbol", Expected: "resources/page/pages.go"},
	{Query: "func (r *resourceSource) MarkStale", Category: "exact_symbol", Expected: "hugolib/content_map.go"},
	{Query: "type OutputFormat struct", Category: "exact_symbol", Expected: "output/outputFormat.go"},
	{Query: "func NewMinifier", Category: "exact_symbol", Expected: "minifiers/minifiers.go"},
	{Query: "func (c *ResourceCache) GetOrCreate", Category: "exact_symbol", Expected: "resources/resource_cache.go"},
	{Query: "func (h *HugoSites) Build", Category: "exact_symbol", Expected: "hugolib/hugo_sites_build.go"},
	{Query: "func (m *pageMeta) newCachedContent", Category: "exact_symbol", Expected: "hugolib/page__content.go"},
	{Query: "func (c *hugoBuilder) build", Category: "exact_symbol", Expected: "commands/hugobuilder.go"},

	// ERROR HANDLING
	{Query: "collecting build errors across multiple sites", Category: "error_handling", Expected: "hugolib/hugo_sites_build.go"},
	{Query: "reporting a syntax error while converting markup", Category: "error_handling", Expected: "markup/goldmark/convert.go"},

	// RENDERING
	{Query: "rendering html output for a page", Category: "rendering", Expected: "hugolib/page__content.go"},
	{Query: "writing minified output assets to the publish dir", Category: "rendering", Expected: "publisher/publisher.go"},

	// CONFIG
	{Query: "applying default configuration values", Category: "config", Expected: "config/config.go"},
	{Query: "reading the site language configuration", Category: "config", Expected: "config/config.go"},
	{Query: "building the flag set for the hugo command", Category: "config", Expected: "commands/commands.go"},
}
