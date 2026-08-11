package main

var muxCases = []BenchCase{
	// CONCEPTUAL
	{Query: "matching incoming requests against registered route patterns", Category: "conceptual", Expected: "mux.go"},
	{Query: "extracting named variables from url path segments", Category: "conceptual", Expected: "route.go"},
	{Query: "restricting a route to specific http methods", Category: "conceptual", Expected: "route.go"},
	{Query: "matching routes by the request host name", Category: "conceptual", Expected: "route.go"},
	{Query: "adding middleware that runs before route handlers", Category: "conceptual", Expected: "middleware.go"},
	{Query: "building nested route groups with a shared path prefix", Category: "conceptual", Expected: "route.go"},
	{Query: "matching on query string parameter pairs", Category: "conceptual", Expected: "route.go"},
	{Query: "redirecting trailing slash requests to the canonical path", Category: "conceptual", Expected: "mux.go"},
	{Query: "matching route paths using regular expressions", Category: "conceptual", Expected: "route.go"},
	{Query: "registering a simple handler function for a path", Category: "conceptual", Expected: "route.go"},

	// EXACT SYMBOL
	{Query: "func NewRouter", Category: "exact_symbol", Expected: "mux.go"},
	{Query: "type Router struct", Category: "exact_symbol", Expected: "mux.go"},
	{Query: "func (r *Router) ServeHTTP", Category: "exact_symbol", Expected: "mux.go"},
	{Query: "func (r *Router) StrictSlash", Category: "exact_symbol", Expected: "mux.go"},
	{Query: "func (r *Router) Walk", Category: "exact_symbol", Expected: "mux.go"},
	{Query: "type Route struct", Category: "exact_symbol", Expected: "route.go"},
	{Query: "func (r *Route) Methods", Category: "exact_symbol", Expected: "route.go"},
	{Query: "func (r *Route) Path", Category: "exact_symbol", Expected: "route.go"},
	{Query: "func (r *Route) Subrouter", Category: "exact_symbol", Expected: "route.go"},
	{Query: "func CORSMethodMiddleware", Category: "exact_symbol", Expected: "middleware.go"},
	{Query: "func (r *Router) Use", Category: "exact_symbol", Expected: "middleware.go"},

	// MIDDLEWARE
	{Query: "applying middleware to a single route only", Category: "request_pipeline", Expected: "route.go"},
	{Query: "chaining middleware across the whole router", Category: "request_pipeline", Expected: "middleware.go"},

	// ERROR HANDLING
	{Query: "detecting when no registered route matches a request", Category: "error_handling", Expected: "mux.go"},
	{Query: "capturing route match errors for debugging", Category: "error_handling", Expected: "route.go"},
}
