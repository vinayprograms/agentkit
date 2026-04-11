// Package agent provides the agent-side ACP implementation.
//
// Create an agent with New, register capability handlers, then call Run:
//
//	srv := agent.New(agent.Config{
//	    Info:         acp.Info{Name: "my-agent", Version: "1.0"},
//	    Capabilities: agent.Capabilities{Image: true},
//	    Prompt: func(ctx context.Context, p prompt.Params) (prompt.Result, error) {
//	        return prompt.Result{Reason: prompt.EndTurn}, nil
//	    },
//	})
//	srv.Run(ctx)
package agent
