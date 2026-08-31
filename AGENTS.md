# AGENTS Policy

## AI-Generated Code Policy

AI agents MUST NOT add `Signed-off-by` tags or otherwise impersonate a human legal attestation. Developer Certificate of Origin (DCO) sign-off is optional in this repository unless a human contributor explicitly chooses to provide it.

AI-generated contributions remain subject to transparent provenance requirements. When performing a `git commit` or submitting a GitHub PR, the commit message or PR description MUST include one of the following tags:

- If the work was **human-assisted by an AI agent**, include:

```
Assisted-by: AGENT_NAME:MODEL_VERSION
```

- If the commit/PR was **fully completed autonomously by an AI agent** (without human authoring), include instead:

```
Autonomously-by: AGENT_NAME:MODEL_VERSION
```

Where:
- `AGENT_NAME` is the name of the AI tool or framework;
- `MODEL_VERSION` is the specific model version used.

Humans remain responsible for reviewing contributions they merge and for any legal or licensing obligations that apply to them. The repository does not require an AI agent to obtain or manufacture a human DCO attestation before technical work can be merged.
