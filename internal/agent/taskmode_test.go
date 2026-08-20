package agent

import "testing"

func TestTaskModeReadOnly(t *testing.T) {
	if !TaskAsk.ReadOnly() || !TaskPlan.ReadOnly() {
		t.Fatal("ask/plan should be read-only")
	}
	if TaskAgent.ReadOnly() || TaskDebug.ReadOnly() {
		t.Fatal("agent/debug should allow writes")
	}
	blocked, _ := TaskAsk.BlockTool("write_file")
	if !blocked {
		t.Fatal("write_file should be blocked in ask mode")
	}
	blocked, _ = TaskAsk.BlockTool("verify")
	if !blocked {
		t.Fatal("verify should be blocked in ask mode")
	}
	blocked, _ = TaskAgent.BlockTool("write_file")
	if blocked {
		t.Fatal("write_file should be allowed in agent mode")
	}
}

func TestParseTaskMode(t *testing.T) {
	if ParseTaskMode("PLAN") != TaskPlan {
		t.Fatal("expected plan")
	}
	if ParseTaskMode("nope") != TaskAgent {
		t.Fatal("expected agent default")
	}
}
