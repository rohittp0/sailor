# Sailor

A terminal dashboard for a user's DigitalOcean account: lists Droplets with their resource Metrics, lets you search/sort, expand a Droplet to see metric charts, SSH into a selected Droplet, and SCP upload local files/folders to it. **Read-only against the DigitalOcean control plane** — it never powers off, reboots, resizes, or destroys Droplets, nor mutates account state via the DO API. SSH and SCP are exec hand-offs that operate *inside* a server the user already controls; they are not control-plane operations.

## Language

**Droplet**:
A DigitalOcean virtual machine in the user's account. The unit listed, sorted, expanded, and connected to.

**Metric**:
A time series of a resource measurement for a Droplet (CPU, memory, disk usage). The "current" value shown in the list is the latest data point of that series.

**Metrics Agent** (`do-agent`):
DigitalOcean's monitoring agent installed on a Droplet. CPU is always available without it; **memory and disk usage Metrics exist only when the agent is installed**. A Droplet without the agent is shown with CPU only and a dim "no agent" indicator; memory/disk render as "n/a".
_Avoid_: "monitoring" (ambiguous with the broader DO Monitoring product)

**Connection Profile**:
The remembered SSH settings for a Droplet — its login user and identity-file path — captured on first SSH and reused (no re-prompt) on every later SSH. **Shared by SSH and SCP**: an upload reuses the same profile, and if none exists yet the SSH prompt captures it first. Stored locally keyed by Droplet ID. Holds the key *path*, never key material, so it contains no secrets. Editable/re-promptable per Droplet.

**Upload** (SCP):
Copying one or more locally-chosen files/folders to the selected Droplet's home directory (`user@ip:`) via the system `scp` binary. The user picks entries in a terminal file/folder chooser (multi-select, cross-directory); nested selections collapse (a selected folder subsumes any separately-selected child). Upload-only, home-directory-only in v1. Writes *inside* the server, not to the DO control plane — see the read-only note above.

## Relationships

- The user's account has many **Droplets**
- A **Droplet** has many **Metrics** (always CPU; memory/disk only with the **Metrics Agent**)
- A **Droplet** has at most one **Connection Profile**

## Blank metrics: two distinct causes

A Droplet can show blank memory/disk (or all-blank) Metrics for two different reasons, surfaced via a status indicator:

- **active + no Metrics Agent** → CPU shown; memory/disk = "n/a (no agent)".
- **off / non-active** (off, building/new, archived) → all Metrics blank with an "off" indicator. Metric API calls are skipped for these Droplets (saves rate limit).

## Flagged ambiguities

- "blank metrics" conflated two causes (powered off vs. no agent) — resolved: distinguished by Droplet status, see above.
