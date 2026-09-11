# Release Packaging Checklist

Before packaging a production build for this change:

1. Start the packaged app with a clean `userData` directory.
   - Expected: the splash runs, then the Hub login page is shown.
   - Expected: no model call is allowed before Hub login and gateway refresh.
2. Log in with an account that has not completed required Hub verification.
   - Expected: the account status page prompts the user to finish verification on `https://analytix.top`.
   - Expected: runtime/model calls remain blocked while `accountReady` is false or gateway is missing.
3. Log in with a verified Hub account.
   - Expected: desktop refreshes the gateway with `/api/desktop/gateway/refresh`.
   - Expected: desktop fetches models from `/v1/models` and builds the managed `analytix-hub` provider from that response.
   - Expected: `/v1/chat/completions` works through the current user's gateway token.
4. Log out from the sidebar account menu.
   - Expected: `/api/desktop/logout` is called.
   - Expected: the gateway token file is removed from `app.getPath("userData")/secrets/`.
   - Expected: the in-memory gateway token is cleared and model calls are blocked again.
5. Verify test bootstrap is absent from the packaged path.
   - Expected: packaged/release builds ignore `ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP` and all `ANALYTIX_HUB_TEST_*` variables.
   - Expected: no test email, password, desktop auth token, or gateway token appears in source, snapshots, `dist`, or unpacked `asar`.
6. Run the release checks for the target platform.
   - Required before packaging: `npm run typecheck`, targeted unit tests, `git diff --check`, and `npm run build`.
   - Platform packaging: `npm run dist:mac`, `npm run dist:win`, or `npm run dist:linux` as appropriate.
