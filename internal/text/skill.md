# {name}: notifications for the user's own chats

## Critical rules

Use {name} to notify the user through their agent, not to message other people.
Send only when the user requested a notification, a long task has finished, or work needs their attention.
Keep messages short and plain text. Never include secrets, credentials or private data.
Never request, type, store or repeat a connect key, token or passphrase.
Never read or print config.toml or the credentials file to diagnose a connection.
Never run {name} revoke-all or {name} connect --store unsafe without the user's explicit request.

## Check readiness

Run {name} status --offline first. It sends no requests and writes no state; it reports local readiness and the last backend check time, not server-side token validity.
If it exits 3 or reports not connected, locked or expired, stop and ask the user to fix the connection in their own terminal.
For a new or expired connection, the user runs {name} connect with a fresh key themselves.
For a locked passphrase connection, ask the user to apply {unlock} in the terminal that launches the agent, then resume from an environment that inherits the unlocked token.
Do not try to answer secret prompts: agents have no interactive terminal for this flow.
For backend diagnostics, the user may run {name} status without --offline. It records the check time for that backend. Its exit code reports local readiness, not backend health.

## Send a notification

{name} notify "Build finished"
echo "Build finished" | {name} notify
{name} notify --platform telegram "Build finished"
{name} notify --platform whatsapp "Build finished"
{name} notify --chat "<chat-id>" "Build finished"

Choose --chat or --platform, never both. Run {name} chats to list valid chat IDs.
Flags override environment defaults, which override the stored default.
THE_AGENT_CLI_DEFAULT_CHAT takes precedence over THE_AGENT_CLI_DEFAULT_PLATFORM.
Without a target, the backend uses the user's latest private chat.
{name} chats default shows the effective target offline.
Only on the user's request, change it with {name} chats default --set "<chat-id>", --platform telegram or --platform whatsapp, or --clear.
Success is silent. Use --verbose only when delivery details are needed.
An argument must be one quoted message. With no argument, stdin is read.
Messages are limited to 4096 characters; oversized stdin is truncated with a warning, but oversized arguments fail.
Do not claim delivery unless the command exits 0.

## Handle failures

Check whether the command succeeded: success may be silent, and warnings do not imply failure.
On failure, read the error and use command help if needed. Do not blindly resend: delivery may already have occurred.
Ask the user to resolve connection, chat-access or credit problems. Stop on rate limits; never retry in a loop.

## Discover current behavior

Run {name} <command> --help for current usage.
Run {name} env to inspect supported environment settings.
