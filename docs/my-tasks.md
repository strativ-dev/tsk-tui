# My Tasks API

Read-only, **user-scoped** API returning the authenticated user's assigned
tasks. Intended for the employee-facing frontend and personal integrations,
where the caller is a real Odoo user reading their own task list.

One endpoint:

| Endpoint | Module |
|----------|--------|
| `GET /api/v1/tasks/my` | `serp_timesheets` |

---

## Authentication

Authenticates the **caller as an Odoo user** via a native Odoo API key sent as
a Bearer token (same `bearer` method as the
[Hour Log Summary API](./hour-log-summary.md)). The request runs as the user
who owns the key, and only that user's tasks are returned.

```
Authorization: Bearer <your-odoo-api-key>
```

Generate the key from Odoo: **Preferences → Account Security → New API Key**.

| Status | Meaning |
|--------|---------|
| `401 Unauthorized` | Missing/malformed `Authorization: Bearer` header, or invalid/revoked API key |

---

## Conventions

- "My tasks" = tasks where the authenticated user is an **assignee**
  (`project.task.user_ids`).
- **Open tasks only by default** — tasks whose stage is not "done"
  (`scrum_stage != "done"`). Pass `include_closed=true` for all.
- Success responses use the envelope `{"data": [...], "count": N}` where
  `count` equals the length of `data`.
- Relational fields render as `{ "id", "name" }` objects, or `null` when unset.
- Tasks are ordered by project, then task number.

---

## Endpoint

### List my tasks

```
GET /api/v1/tasks/my?include_closed=false
Authorization: Bearer <your-odoo-api-key>
```

Returns the authenticated user's assigned tasks.

**Query parameters**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `include_closed` | boolean | no | `true`/`1` returns all assigned tasks, done stages included. Default `false` (open tasks only) |

**Response `200 OK`**

```json
{
  "count": 1,
  "data": [
    {
      "id": 1372,
      "key": "SE360-1372",
      "name": "Add hour-log summary API",
      "project": { "id": 42, "name": "ERP 360" },
      "stage": { "id": 7, "name": "In Progress", "stage_type": "doing" },
      "priority": "2",
      "deadline": "2026-07-31",
      "sprint": { "id": 15, "name": "Sprint 24" },
      "epic": { "id": 3, "name": "Public APIs" },
      "billing_status": "billable",
      "short_url": "https://erp.strativ.se/tasks/SE360-1372",
      "hours_spent": 12.5
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Odoo `project.task` id |
| `key` | string \| null | Human task key `<project short_code>-<number>` (e.g. `SE360-1372`); `null` if not yet computed |
| `name` | string | Task title |
| `project` | object \| null | `{ id, name }` of the task's project |
| `stage` | object \| null | `{ id, name, stage_type }`; `stage_type` is one of `todo`, `doing`, `done` |
| `priority` | string | Task priority level: `0` Low, `1` Medium, `2` High, `3` Highest |
| `deadline` | string \| null | Task deadline, `YYYY-MM-DD`; `null` if unset |
| `sprint` | object \| null | `{ id, name }` of the sprint; `null` if none |
| `epic` | object \| null | `{ id, name }` of the epic; `null` if none |
| `billing_status` | string \| null | `billable` or `non_billable` |
| `short_url` | string \| null | Shareable task URL |
| `hours_spent` | number | Hours logged directly on the task (excludes subtasks), rounded to 2 decimals |

**Error responses**

| Status | When |
|--------|------|
| `401 Unauthorized` | Bad or missing Bearer key |

---

## Notes

- Read-only; the endpoint never writes tasks.
- Scoped to the token owner only — tasks assigned to other users are excluded.
- List shape: no subtasks, description, or per-task hours (use the customer
  portal task-detail endpoint for the full record).
- Query logic lives in the model method
  `project.task.get_my_tasks(user, include_closed=False)` (`serp_timesheets`),
  so it is reusable and unit-tested independently of HTTP.
