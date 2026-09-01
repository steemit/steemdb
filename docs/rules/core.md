# Core Conventions (always apply)

## Conversation Language Policy

- **Let the user decide the conversation language at the start of each new chat**
- **Default to Chinese (中文) if no preference is specified**
- Ask the user: "Which language would you prefer for our conversation? (Default: 中文)"
- Respect the user's choice throughout the entire chat session
- This ensures comfortable communication for the user

## Code Documentation and Comments

- All code comments, documentation, and rules must be written in English.
- This includes:
  - Inline code comments
  - Function/class documentation (JSDoc comments)
  - README files and other documentation
  - Code review comments
  - Error messages visible to developers
  - Configuration file comments

Rationale: using English for all technical documentation ensures consistency
across the codebase, better collaboration with international developers,
easier maintenance and onboarding, and matches standard practice in open
source projects.

## Git Commit Policy

- **Never automatically commit changes without explicit user confirmation**
- **All git commit messages must be written in English**
- Before creating a commit:
  1. Show the user what files will be committed (`git status`)
  2. Present the proposed commit message
  3. Wait for explicit user approval before executing `git commit`
- Only use `git add` and prepare commits, but always ask for confirmation
  before committing
- This ensures the user maintains full control over the git history and commit
  timing

## Network Issue Handling Policy

- **If network-related errors occur, pause and notify the user to fix the
  proxy server**
- When encountering network errors (e.g. Docker pull failures, connection
  timeouts, DNS resolution failures, proxy authentication failures):
  1. **Stop immediately** — do not retry automatically
  2. **Identify the error** — clearly state what network operation failed
  3. **Notify the user** — inform them that network/proxy issues need to be
     resolved
  4. **Wait for user confirmation** — only proceed after the user confirms
     the network issue is fixed
- Common scenarios: Docker image pull failures, package download failures
  (`go get`, `npm install`, etc.), API connection timeouts, DNS resolution
  failures

## Documentation Update Policy

- **Always update related documentation when completing TODO tasks**
- When marking TODO items as completed:
  1. Review all related documentation files that may need updates
  2. Update README files, API documentation, and usage guides as needed
  3. Ensure documentation reflects the current state of the codebase
  4. Update version numbers, examples, and feature lists if applicable
- Documentation updates should be part of the same workflow as completing the
  actual coding task

## Package Management

See [project.md](project.md) for project-specific package management rules.
