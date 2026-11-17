package mcp

import "encoding/json"

// Call runs a tool as an agent's call would: its answer is text.
func (s *Server) Call(name string, args json.RawMessage) (string, error) { return s.call(name, args) }
