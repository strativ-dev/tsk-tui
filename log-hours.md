# Log Hours API

**User-scoped write** API creating one timesheet entry (`account.analytic.line`)
for the authenticated user against a task, identified by the task's key. The
read counterpart is the [Hour Log Summary API](./hour-log-summary.md).

One endpoint:

| Endpoint | Module |
|----------|--------|
| `POST /api/v1/timesheets/log` | `serp_timesheets` |

---

## Authentication

Authenticates the **caller as an Odoo user** via a native Odoo API key sent as
a Bearer token (same `bearer` method as the other timesheet endpoints). The
entry is logged for the caller's own employee.

```
Authorization: Bearer <your-odoo-api-key>
```

| Status | Meaning |
|--------|---------|
| `401 Unauthorized` | Missing/malformed `Authorization: Bearer` header, or invalid/revoked API key |

---

## Endpoint

### Log one entry

```
POST /api/v1/timesheets/log
Authorization: Bearer <your-odoo-api-key>
Content-Type: application/json
```

**Request body**

```json
{
  "task_key": "SE360-1372",
  "date": "2026-07-10",
  "hours": 5.5,
  "description": "Implemented the hour-log API"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `task_key` | string | yes | Task key `<SHORT_CODE>-<NUMBER>` (e.g. `SE360-1372`) |
| `date` | string | yes | Log date, `YYYY-MM-DD` |
| `hours` | number | yes | Hours to log; must be `> 0` and `<= 24` |
| `description` | string | yes | Work description; stored HTML-escaped |

**Response `201 Created`**

```json
{
  "id": 90211,
  "task_key": "SE360-1372",
  "date": "2026-07-10",
  "hours": 5.5,
  "description": "Implemented the hour-log API"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Created `account.analytic.line` id |
| `task_key` | string | Echoed task key |
| `date` | string | Echoed log date, `YYYY-MM-DD` |
| `hours` | number | Logged hours (`unit_amount`) |
| `description` | string | Echoed description |

**Error responses** — `{"error": "<message>", "code": "<code>"}`

| Status | `code` | When |
|--------|--------|------|
| `400` | `no_employee` | The authenticated user has no linked employee |
| `400` | `no_hourly_cost` | The caller's employee has no hourly cost set (contact HR) |
| `400` | `task_key_format` | `task_key` is not `<SHORT_CODE>-<NUMBER>` |
| `400` | `invalid_date` | `date` is missing or not `YYYY-MM-DD` |
| `400` | `invalid_hours` | `hours` is missing, non-numeric, `<= 0`, or `> 24` |
| `400` | `invalid_description` | `description` is missing or blank |
| `400` | `daily_cap_exceeded` | This log would push the day's total above 24h |
| `404` | `task_not_found` | No task matches `task_key` |
| `409` | `task_ambiguous` | `task_key` matches tasks in more than one project; body also carries `candidates: [{id, name, project}]` |
| `500` | — | Unexpected error |

---

## Notes

- Logs for the token owner's employee only — you cannot log for another user.
- One entry per request (no batch); retrying a timed-out request may create a
  duplicate line — do not blindly retry.
- Limits mirror the MCP `log_hours` tool: max 24h per entry and 24h per day.
- The entry is created unconfirmed (`confirmed = false`).
- Logic lives in the model method
  `account.analytic.line.log_hour_for_user(user, task_key, date, hours, description)`
  (`serp_timesheets`), reused and unit-tested independently of HTTP.
