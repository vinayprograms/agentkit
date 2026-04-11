// Package host provides the host-side (editor/IDE) ACP implementation.
//
// Create a host with New, provide capability handlers, then call Run:
//
//	h := host.New(host.Config{
//	    Info:         acp.Info{Name: "my-editor", Version: "2.0"},
//	    Capabilities: host.Capabilities{Terminal: true, ReadTextFile: true},
//	    Permission:   handlePermission,
//	    ReadFile:     handleReadFile,
//	})
//	h.Run(ctx)
package host
