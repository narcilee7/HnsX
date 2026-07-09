// Package mcp provides a Model Context Protocol client.
package mcp

// Client connects to MCP servers.
type Client struct{}

// NewClient creates a new MCP client.
func NewClient() *Client {
	return &Client{}
}
