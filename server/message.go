package main

type ProtocolMessageType int

const (
	RegisterRequest ProtocolMessageType = iota //REGISTER file
	FileListRequest //get list of files
	FileLocationsRequest // locations of chunks:chunkid for file
	ChunkRegisterRequest //
	FileChunkRequest
	NoneType
)

/*steps:
peer sends chunk reg request for file, server responds with peer(s) that can store chunk*/