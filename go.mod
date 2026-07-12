module github.com/spooled-cloud/spooled-sdk-go

go 1.25.0

// v1.0.10 had release activity on more than one tag target, making its
// provenance ambiguous after the pushed tag was moved. Keep it retracted and
// use v1.0.11 or newer; never move an externally visible module tag.
retract v1.0.10

require (
	github.com/coder/websocket v1.8.15
	github.com/oapi-codegen/runtime v1.4.0
	google.golang.org/grpc v1.81.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260504160031-60b97b32f348 // indirect
)
