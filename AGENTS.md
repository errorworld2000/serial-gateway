# Delivery workflow

After completing a requested update:

1. Run the relevant checks and commit the completed changes.
2. Build the committed version with `scripts/build.ps1`, using its default output `build/serial-gateway.exe`.
3. Stop the previous instance from this project's build directory if needed, then start the newly built executable.
4. Verify that the service started successfully.

Always use `serial-gateway.exe`; do not create differently named application executables such as `serial-gateway-commands.exe`. Preserve `build/serial-gateway.json` when updating.
