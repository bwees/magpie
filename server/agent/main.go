// Command agent runs GPU jobs for the server. It ships as a separate image.
package main

import "magpie/internal/transcode/api/agentproto"

// The agent speaks the server's control-channel protocol; its first frame is a Hello.
var _ = agentproto.TypeHello

func main() {}
