# Review Guidelines — dotfiles-mcp

Inherits from org-wide [REVIEW.md](https://github.com/hairglasses-studio/.github/blob/main/REVIEW.md).

## Additional Focus
- **Compositor abstraction**: Tools must work on both Hyprland and Sway — check for compositor-specific assumptions
- **Shader file I/O**: Validate paths before read/write, prevent path traversal outside shader directories
- **Bluetooth/MIDI**: Handle device disconnect gracefully, don't panic on missing devices
- **Atomic config writes**: Use `mktemp + mv` pattern for Ghostty configs (prevents partial reads)
- **Mirror contract**: Any public-surface change must keep `.well-known/mcp.json` and `snapshots/contract/*` in sync
- **Canonical drift**: Changes should not accidentally widen the gap from `dotfiles/mcp/dotfiles-mcp` without an explicit reason
