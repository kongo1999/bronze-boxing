package server

// failpoint lets integration tests inject a failure at a named step inside a
// multi-document write, to prove the whole change rolls back. It is a no-op
// in production: nothing outside tests ever replaces it.
var failpoint = func(name string) error { return nil }
