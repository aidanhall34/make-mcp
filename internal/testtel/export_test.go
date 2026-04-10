package testtel

// InitInstrumentsForTest exposes initInstruments for use in external test
// files. Tests set up an in-memory meter provider, then call this to create
// instruments on that provider so RecordToolsListRequest and Start can be
// tested without a live OTLP endpoint.
var InitInstrumentsForTest = initInstruments
