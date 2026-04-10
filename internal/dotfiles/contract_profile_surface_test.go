package dotfiles

import "testing"

func contractDeferredMap(t *testing.T, profile string) map[string]bool {
	t.Helper()
	bundle, err := BuildContractSnapshotBundle(profile)
	if err != nil {
		t.Fatalf("BuildContractSnapshotBundle(%s): %v", profile, err)
	}
	deferred := make(map[string]bool, len(bundle.Tools))
	for _, tool := range bundle.Tools {
		deferred[tool.Name] = tool.Deferred
	}
	return deferred
}

func TestContractSnapshotDesktopProfileSurfaceBoundaries(t *testing.T) {
	deferred := contractDeferredMap(t, "desktop")

	for _, tool := range []string{
		"dotfiles_desktop_status",
		"dotfiles_workstation_diagnostics",
		"dotfiles_workspace_scene",
		"desktop_snapshot",
		"desktop_wait_for_element",
		"session_screenshot",
		"session_clipboard_get",
		"session_find_ui_element",
		"kitty_show_image",
		"notification_history_entries",
		"input_status",
	} {
		if deferred[tool] {
			t.Fatalf("%s should be eager in desktop profile", tool)
		}
	}

	for _, tool := range []string{
		"bt_list_devices",
		"archwiki_search",
		"system_info",
		"ps_list",
	} {
		if !deferred[tool] {
			t.Fatalf("%s should remain deferred in desktop profile", tool)
		}
	}
}

func TestContractSnapshotOpsProfileSurfaceBoundaries(t *testing.T) {
	deferred := contractDeferredMap(t, "ops")

	for _, tool := range []string{
		"dotfiles_server_health",
		"dotfiles_desktop_status",
		"dotfiles_workstation_diagnostics",
		"dotfiles_workspace_scene",
		"workflow_sync",
		"archwiki_search",
		"arch_news_latest",
	} {
		if deferred[tool] {
			t.Fatalf("%s should be eager in ops profile", tool)
		}
	}

	for _, tool := range []string{
		"desktop_snapshot",
		"session_connect",
		"kitty_status",
		"input_status",
	} {
		if !deferred[tool] {
			t.Fatalf("%s should remain deferred in ops profile", tool)
		}
	}
}
