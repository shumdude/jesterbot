package botconfig

import "testing"

func TestLoad(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Scenes.Scenes) == 0 {
		t.Fatal("expected tgaml scenes to be loaded")
	}
	if len(cfg.Scenes.Commands) == 0 {
		t.Fatal("expected tgaml commands to be loaded")
	}
	if len(cfg.Keyboards.Keyboards) == 0 {
		t.Fatal("expected tgaml keyboards to be loaded")
	}
}
