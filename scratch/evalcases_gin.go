package main

var ginCases = []BenchCase{
	// CONCEPTUAL — natural language "I'm looking for X" queries
	{Query: "where is the HTTP request router tree matching logic", Category: "conceptual", Expected: "tree.go"},
	{Query: "how to handle JSON body binding and validation errors", Category: "conceptual", Expected: "binding/json.go"},
	{Query: "recovering from panics and printing stack trace middleware", Category: "conceptual", Expected: "recovery.go"},
	{Query: "writing colored HTTP access logs to stdout or file", Category: "conceptual", Expected: "logger.go"},
	{Query: "parsing multiparts file upload and saving to disk", Category: "conceptual", Expected: "context.go"},
	{Query: "cross origin resource sharing CORS header settings", Category: "conceptual", Expected: "mode.go"},
	{Query: "where do we render HTML templates and static files", Category: "conceptual", Expected: "render/html.go"},
	{Query: "serving static assets and directory index files", Category: "conceptual", Expected: "fs.go"},
	{Query: "setting response cookie with security attributes", Category: "conceptual", Expected: "context.go"},
	{Query: "how to abort request pipeline processing early", Category: "conceptual", Expected: "context.go"},
	{Query: "where are custom validator tag rules registered", Category: "conceptual", Expected: "binding/default_validator.go"},
	{Query: "protobuf protocol buffer response rendering", Category: "conceptual", Expected: "render/protobuf.go"},
	{Query: "YAML payload request binding validator", Category: "conceptual", Expected: "binding/yaml.go"},
	{Query: "how to redirect incoming HTTP request to another URL", Category: "conceptual", Expected: "context.go"},
	{Query: "where is gin release mode debug mode configured", Category: "conceptual", Expected: "mode.go"},
	{Query: "validating incoming form field values against struct tags", Category: "conceptual", Expected: "binding/form_mapping.go"},

	// EXACT SYMBOL — developer typing the literal identifier
	{Query: "Engine struct definition", Category: "exact_symbol", Expected: "gin.go"},
	{Query: "Context struct definition", Category: "exact_symbol", Expected: "context.go"},
	{Query: "func (engine *Engine) Run", Category: "exact_symbol", Expected: "gin.go"},
	{Query: "func (c *Context) JSON", Category: "exact_symbol", Expected: "context.go"},
	{Query: "func (c *Context) BindJSON", Category: "exact_symbol", Expected: "context.go"},
	{Query: "type HandlerFunc func(*Context)", Category: "exact_symbol", Expected: "gin.go"},
	{Query: "func RecoveryWithWriter", Category: "exact_symbol", Expected: "recovery.go"},
	{Query: "func LoggerWithConfig", Category: "exact_symbol", Expected: "logger.go"},
	{Query: "type RouterGroup struct", Category: "exact_symbol", Expected: "routergroup.go"},
	{Query: "func (group *RouterGroup) GET", Category: "exact_symbol", Expected: "routergroup.go"},
	{Query: "func (c *Context) Abort", Category: "exact_symbol", Expected: "context.go"},
	{Query: "func Default() *Engine", Category: "exact_symbol", Expected: "gin.go"},
	{Query: "func (engine *Engine) Use", Category: "exact_symbol", Expected: "gin.go"},

	// ERROR HANDLING
	{Query: "collecting and rendering multiple request errors as a JSON payload", Category: "error_handling", Expected: "errors.go"},
	{Query: "aborting the handler chain and writing an error status immediately", Category: "error_handling", Expected: "context.go"},

	// REQUEST PIPELINE
	{Query: "reading the raw request body bytes before binding", Category: "request_pipeline", Expected: "context.go"},
	{Query: "sending the response header and status exactly once", Category: "request_pipeline", Expected: "response_writer.go"},
	{Query: "checking whether the client has aborted the connection mid-request", Category: "request_pipeline", Expected: "context.go"},
	{Query: "hijacking the underlying connection for raw websocket streaming", Category: "request_pipeline", Expected: "response_writer.go"},

	// RENDERING / RESPONSE FORMATS
	{Query: "rendering XML responses from Go structs", Category: "rendering", Expected: "render/xml.go"},
	{Query: "sending a file download as an inline attachment", Category: "rendering", Expected: "context.go"},
	{Query: "rendering responses with pretty-printed indented JSON", Category: "rendering", Expected: "context.go"},
	{Query: "returning raw binary bytes with a custom content type", Category: "rendering", Expected: "render/data.go"},

	// CONFIG / SETUP
	{Query: "creating the engine preloaded with logger and recovery middleware", Category: "config", Expected: "gin.go"},
	{Query: "configuring trusted proxy headers to preserve the client IP", Category: "config", Expected: "gin.go"},
	{Query: "turning off debug log noise when running tests", Category: "config", Expected: "mode.go"},
}
