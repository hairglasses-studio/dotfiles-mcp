# Review Guidelines — dotfiles-mcp

Inherits from org-wide [REVIEW.md](https://github.com/hairglasses-studio/.github/blob/main/REVIEW.md).

## Additional Focus
- **Compositor abstraction**: Tools must work on both Hyprland and Sway — check for compositor-specific assumptions
- **Shader file I/O**: Validate paths before read/write, prevent path traversal outside shader directories
- **Bluetooth/MIDI**: Handle device disconnect gracefully, don't panic on missing devices
- **Atomic config writes**: Use `mktemp + mv` pattern for Ghostty configs (prevents partial reads)
- **Large tool surface**: Keep tool names action-oriented, one clear intent per tool, and preserve discovery-first usability as the contract grows
