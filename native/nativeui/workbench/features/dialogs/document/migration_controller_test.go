package documentdialogs

import "testing"

func TestMigrationControllerDispatchesApplyToCompositionRoot(t *testing.T) {
	handle := NewHandle()
	handle.OpenCoreMigration()
	applyCalls := 0
	closeCalls := 0
	controller := NewMigrationController(handle, MigrationActions{
		Apply: func() { applyCalls++ },
		Close: func() { closeCalls++ },
	})

	controller.HandleAction(Action{Kind: ApplyCoreMigration})

	if applyCalls != 1 || closeCalls != 0 || !handle.ModalOpen() {
		t.Fatalf("apply action calls = (%d, %d), modal open = %v", applyCalls, closeCalls, handle.ModalOpen())
	}
}

func TestMigrationControllerClosesFeatureStateAndNotifiesRoot(t *testing.T) {
	handle := NewHandle()
	handle.OpenCoreMigration()
	closeCalls := 0
	controller := NewMigrationController(handle, MigrationActions{
		Close: func() { closeCalls++ },
	})

	controller.HandleAction(Action{Kind: CloseCoreMigration})

	if handle.ModalOpen() || closeCalls != 1 {
		t.Fatalf("close calls = %d, modal open = %v", closeCalls, handle.ModalOpen())
	}
}

func TestMigrationControllerIgnoresUnrelatedActions(t *testing.T) {
	handle := NewHandle()
	applyCalls := 0
	closeCalls := 0
	controller := NewMigrationController(handle, MigrationActions{
		Apply: func() { applyCalls++ },
		Close: func() { closeCalls++ },
	})

	controller.HandleAction(Action{Kind: TryMerge})
	if applyCalls != 0 || closeCalls != 0 {
		t.Fatalf("unrelated action invoked callbacks: apply = %d, close = %d", applyCalls, closeCalls)
	}
}
