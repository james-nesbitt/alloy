package modal

import (
	"testing"
)

func TestModalEngine(t *testing.T) {
	driver := NewVimDriver()
	engine := NewEngine(driver)

	t.Run("Vim Normal to Insert", func(t *testing.T) {
		intent, consumed := engine.Process(Key{Code: "i"})
		if !consumed {
			t.Error("Vim didn't consume 'i' in normal mode")
		}
		if mi, ok := intent.(ModeIntent); !ok || mi.NewMode != ModeInsert {
			t.Errorf("Expected ModeIntent(Insert), got %v", intent)
		}
		if engine.State.Mode != ModeInsert {
			t.Errorf("Expected state to change to Insert, got %v", engine.State.Mode)
		}
	})

	t.Run("Vim Insert to Normal", func(t *testing.T) {
		intent, consumed := engine.Process(Key{Code: "esc"})
		if !consumed {
			t.Error("Vim didn't consume 'esc' in insert mode")
		}
		if mi, ok := intent.(ModeIntent); !ok || mi.NewMode != ModeNormal {
			t.Errorf("Expected ModeIntent(Normal), got %v", intent)
		}
		if engine.State.Mode != ModeNormal {
			t.Errorf("Expected state to back to Normal, got %v", engine.State.Mode)
		}
	})

	t.Run("Vim Movement", func(t *testing.T) {
		intent, consumed := engine.Process(Key{Code: "j"})
		if !consumed {
			t.Error("Vim didn't consume 'j' in normal mode")
		}
		if move, ok := intent.(MoveIntent); !ok || move.Direction != "down" {
			t.Errorf("Expected MoveIntent(down), got %v", intent)
		}
	})

	t.Run("Vim Multi-key Sequence (gg)", func(t *testing.T) {
		intent, consumed := engine.Process(Key{Code: "g"})
		if !consumed || intent != nil {
			t.Errorf("Expected consumed=true, intent=nil for first 'g', got %v, %v", consumed, intent)
		}
		if len(engine.State.Pending) != 1 {
			t.Errorf("Expected pending buffer size 1, got %d", len(engine.State.Pending))
		}

		intent, consumed = engine.Process(Key{Code: "g"})
		if !consumed || intent == nil {
			t.Errorf("Expected consumed=true, intent!=nil for second 'g', got %v, %v", consumed, intent)
		}
		if move, ok := intent.(MoveIntent); !ok || move.Direction != "buffer-start" {
			t.Errorf("Expected MoveIntent(buffer-start), got %v", intent)
		}
		if len(engine.State.Pending) != 0 {
			t.Errorf("Expected pending buffer cleared, got %d", len(engine.State.Pending))
		}
	})

	t.Run("Vim Multi-key Sequence (dd)", func(t *testing.T) {
		intent, consumed := engine.Process(Key{Code: "d"})
		if !consumed || intent != nil {
			t.Errorf("Expected consumed=true, intent=nil for first 'd', got %v, %v", consumed, intent)
		}

		intent, consumed = engine.Process(Key{Code: "d"})
		if !consumed || intent == nil {
			t.Errorf("Expected consumed=true, intent!=nil for second 'd', got %v, %v", consumed, intent)
		}
		if action, ok := intent.(ActionIntent); !ok || action.Verb != "delete-line" {
			t.Errorf("Expected ActionIntent(delete-line), got %v", intent)
		}
	})

	t.Run("Vim Sequence with Count (3j)", func(t *testing.T) {
		engine.Process(Key{Code: "3"})
		intent, consumed := engine.Process(Key{Code: "j"})
		if !consumed {
			t.Error("Vim didn't consume 'j' after '3'")
		}
		if move, ok := intent.(MoveIntent); !ok || move.Count != 3 || move.Direction != "down" {
			t.Errorf("Expected MoveIntent(down, 3), got %v", intent)
		}
	})
}

func TestHelixDriver(t *testing.T) {
	driver := NewHelixDriver()
	engine := NewEngine(driver)

	t.Run("Helix Selection-First", func(t *testing.T) {
		// Selection is always active in Helix philosophy
		intent, consumed := engine.Process(Key{Code: "j"})
		if !consumed {
			t.Error("Helix didn't consume 'j' in normal mode")
		}
		if move, ok := intent.(MoveIntent); !ok || move.Direction != "down" {
			t.Errorf("Expected MoveIntent(down), got %v", intent)
		}
	})

	t.Run("Helix Multi-key Sequence (gg)", func(t *testing.T) {
		intent, consumed := engine.Process(Key{Code: "g"})
		if !consumed || intent != nil {
			t.Errorf("Expected consumed=true, intent=nil for first 'g', got %v, %v", consumed, intent)
		}
		if len(engine.State.Pending) != 1 {
			t.Errorf("Expected pending buffer size 1, got %d", len(engine.State.Pending))
		}

		intent, consumed = engine.Process(Key{Code: "g"})
		if !consumed || intent == nil {
			t.Errorf("Expected consumed=true, intent!=nil for second 'g', got %v, %v", consumed, intent)
		}
		if move, ok := intent.(MoveIntent); !ok || move.Direction != "buffer-start" {
			t.Errorf("Expected MoveIntent(buffer-start), got %v", intent)
		}
	})

	t.Run("Helix Count Sequence (3w)", func(t *testing.T) {
		engine.Process(Key{Code: "3"})
		intent, consumed := engine.Process(Key{Code: "w"})
		if !consumed {
			t.Error("Helix didn't consume 'w' after '3'")
		}
		if move, ok := intent.(MoveIntent); !ok || move.Count != 3 || move.Direction != "word-forward" {
			t.Errorf("Expected MoveIntent(word-forward, 3), got %v", intent)
		}
	})

	t.Run("Helix Action (delete)", func(t *testing.T) {
		intent, consumed := engine.Process(Key{Code: "d"})
		if !consumed {
			t.Error("Helix didn't consume 'd'")
		}
		if action, ok := intent.(ActionIntent); !ok || action.Verb != "delete" {
			t.Errorf("Expected ActionIntent(delete), got %v", intent)
		}
	})
}
