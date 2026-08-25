# Headless v3 contract

`picogent run` is intended for scripts and CI as well as a terminal. Its
contract is deliberately small:

| Exit | Meaning |
| ---: | --- |
| `0` | The turn returned without an execution error and did not leave required work unverified. |
| `1` | Provider, tool, configuration, or other execution failure. |
| `2` | The run is blocked, including a denied approval or a durable task blocker. |
| `3` | The run returned, but required completion or mutation evidence is not verified. |
| `130` | The turn was interrupted by cancellation or SIGINT/SIGTERM. |

Successful assistant text is written to stdout. Tool progress, permission
prompts, and diagnostics are written to stderr, so a script can capture the
answer without accidentally treating a prompt or error as answer content.

Safe mode prompts for actions that need approval. `--yes` approves only
non-destructive actions within the selected workspace; destructive and
out-of-workspace actions remain blocked. A blocked run keeps its durable task
state so rerunning the same prompt can continue it.

When `--clarify` is supplied, answering `esc` or `cancel` is an explicit
cancellation and returns exit `130`; it is not reported as a successful run.

For an inferred or explicit completion goal, exit `0` requires the agent's
completion marker and passing verification evidence. A successful model reply
alone is not completion proof. One-shot output does not advertise `/undo`:
interactive undo checkpoints are process-local and are not persisted by the
headless command.

SIGINT and SIGTERM cancel the same context used by the model and tools. The
command exits `130`; rerunning the same prompt resumes from the saved task
checkpoint when one exists.
