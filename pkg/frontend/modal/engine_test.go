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

    t.Run("Helix Action-Selection", func(t *testing.T) {
        intent, consumed := engine.Process(Key{Code: "x"})
        if !consumed {
            t.Error("Helix didn't consume 'x' in normal mode")
        }
        if action, ok := intent.(ActionIntent); !ok || action.Verb != "select-line" {
            t.Errorf("Expected ActionIntent(select-line), got %v", intent)
        }
    })
}
