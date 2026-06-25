# slack-mirror

Mirror the **current state** of selected Slack channels into Postgres so the content
stays searchable by a person or an LLM agent after Slack's free-tier 90-day window
hides it. This is a *mirror, not an archive*: edits update rows, deletes remove rows.

Generic and business-agnostic — configured entirely via environment variables.

Status: under construction. See full docs below as milestones land.
