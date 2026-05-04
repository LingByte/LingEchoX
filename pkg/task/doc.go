// Package task provides a priority scheduler with a fixed worker pool for SIP campaign dialing
// and similar workloads.
//
// SubmitTask pushes onto an in-memory priority queue (higher priority first). Workers block on
// sync.Cond and pop directly from that queue — there is no secondary worker channel backlog.
//
// Each Task exposes QueueAhead, QueuedTotal, RunningWorkers, and UnfinishedEstimate snapshots
// taken at enqueue/relabel time so callers can log queue position and rough system load.
//
// Use Wait() to block until a task completes; completion does not use exported channels.
package task
