// Package a2a implements the A2A (Agent-to-Agent) protocol adapter using the
// official a2a-go SDK (github.com/a2aproject/a2a-go v0.3.7).
//
// Verified SDK interfaces (v0.3.7):
//
//	a2asrv.AgentExecutor:
//	  Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error
//	  Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error
//
//	a2asrv.TaskStore:
//	  Save(ctx context.Context, task *a2a.Task, event a2a.Event, prev a2a.TaskVersion) (a2a.TaskVersion, error)
//	  Get(ctx context.Context, id a2a.TaskID) (*a2a.Task, a2a.TaskVersion, error)
//	  List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error)
//
//	a2asrv.AgentCardProducer:
//	  Card(ctx context.Context) (*a2a.AgentCard, error)
//
//	eventqueue.Queue:
//	  Write(ctx context.Context, event a2a.Event) error
//
//	a2asrv.NewHandler(executor AgentExecutor, opts ...RequestHandlerOption) RequestHandler
//	a2asrv.NewJSONRPCHandler(handler RequestHandler, opts ...JSONRPCHandlerOption) http.Handler
//	a2asrv.WithTaskStore(store TaskStore) RequestHandlerOption
//	a2asrv.WithExtendedAgentCardProducer(producer AgentCardProducer) RequestHandlerOption
//	a2asrv.NewAgentCardHandler(producer AgentCardProducer) http.Handler
package a2a
