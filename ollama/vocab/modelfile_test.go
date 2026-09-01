package vocab

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	got, err := Render("qwen2.5:14b")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "FROM qwen2.5:14b") {
		t.Errorf("rendered Modelfile missing FROM line for base model:\n%s", got)
	}
	if strings.Contains(got, "{{.BaseModel}}") {
		t.Errorf("rendered Modelfile still contains unrendered placeholder:\n%s", got)
	}
}

func TestRenderDefaultBaseModel(t *testing.T) {
	got, err := Render(DefaultBaseModel)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "FROM "+DefaultBaseModel) {
		t.Errorf("rendered Modelfile missing FROM line for default base model:\n%s", got)
	}
}
