# Delivery workflow

Preserve the existing Nothing-inspired visual style: black surfaces, monochrome dot matrix, square borders, restrained red accents. Motion should reinforce this style; avoid colored auroras, neon glows, glass panels, and standalone decorative 3D widgets.

The user also requested warm Claude Code-inspired terminal colors, a brief holographic sphere unfolding on terminal switches, and a small interactive pet. These are intentional additions to the Nothing base. Do not add an animation toggle; respect the system reduced-motion preference.

After completing a requested update:

1. Run the relevant checks and commit the completed changes.
2. Build the committed version with `scripts/build.ps1`, using its default output `build/serial-gateway.exe`.
3. Stop the previous instance from this project's build directory if needed, then start the newly built executable.
4. Verify that the service started successfully.

Always use `serial-gateway.exe`; do not create differently named application executables such as `serial-gateway-commands.exe`. Preserve `build/serial-gateway.json` when updating.
