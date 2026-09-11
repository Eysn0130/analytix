import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";

export type McpLaunchOptions = {
  baseUrl: string;
  secret: string;
};

function parseArgValue(argv: string[], flag: string): string {
  const index = argv.indexOf(flag);
  if (index < 0) return "";
  return argv[index + 1] ?? "";
}

function parseLaunchOptions(argv: string[]): McpLaunchOptions | null {
  if (
    !argv.includes("--gui-schedule-mcp-server") &&
    !argv.includes("--claw-schedule-mcp-server")
  )
    return null;
  const baseUrl =
    parseArgValue(argv, "--base-url").trim() || "http://127.0.0.1:8787";
  const secret = parseArgValue(argv, "--secret").trim();
  return { baseUrl, secret };
}

async function postJson(
  options: McpLaunchOptions,
  path: string,
  body: Record<string, unknown>,
): Promise<unknown> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (options.secret) {
    headers.Authorization = `Bearer ${options.secret}`;
  }
  const response = await fetch(`${options.baseUrl}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(15_000),
  });
  const text = await response.text();
  let parsed: unknown;
  try {
    parsed = JSON.parse(text) as unknown;
  } catch {
    throw new Error("Schedule backend returned invalid JSON.");
  }
  if (!response.ok) {
    throw new Error(`Schedule backend returned HTTP ${response.status}.`);
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error("Schedule backend returned a non-object response.");
  }
  return parsed;
}

function textResult(text: string, structuredContent?: Record<string, unknown>) {
  return {
    content: [{ type: "text" as const, text }],
    ...(structuredContent ? { structuredContent } : {}),
  };
}

function errorResult(
  message: string,
  structuredContent: Record<string, unknown>,
) {
  return {
    content: [{ type: "text" as const, text: message }],
    structuredContent,
    isError: true,
  };
}

const scheduledTaskOutputSchema = z.strictObject({
  id: z.string(),
  title: z.string(),
  enabled: z.boolean(),
  prompt: z.string(),
  workspaceRoot: z.string(),
  clawChannelId: z.string(),
  providerId: z.string().optional(),
  model: z.string(),
  reasoningEffort: z.enum(["auto", "off", "low", "medium", "high", "max"]),
  mode: z.enum(["agent", "plan"]),
  schedule: z.strictObject({
    kind: z.enum(["manual", "at", "daily", "interval"]),
    everyMinutes: z.number(),
    timeOfDay: z.string(),
    atTime: z.string(),
  }),
  createdAt: z.string(),
  updatedAt: z.string(),
  lastRunAt: z.string(),
  nextRunAt: z.string(),
  lastStatus: z.enum(["idle", "running", "success", "error"]),
  lastMessage: z.string(),
  lastThreadId: z.string(),
});

const outcomeStatusSchema = z.enum(["success", "failure"]);

const atScheduleInputSchema = z.strictObject({
  kind: z.literal("at"),
  atTime: z.string().min(1),
});

const dailyScheduleInputSchema = z.strictObject({
  kind: z.literal("daily"),
  timeOfDay: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/),
});

const intervalScheduleInputSchema = z.strictObject({
  kind: z.literal("interval"),
  everyMinutes: z.number().int().min(1).max(10080),
});

const createScheduleInputSchema = z.discriminatedUnion("kind", [
  atScheduleInputSchema,
  dailyScheduleInputSchema,
  intervalScheduleInputSchema,
]);

const updateScheduleInputSchema = z.union([
  z.strictObject({ kind: z.literal("manual") }),
  atScheduleInputSchema,
  dailyScheduleInputSchema,
  intervalScheduleInputSchema,
]);

const listBackendResponseSchema = z.strictObject({
  ok: z.literal(true),
  tasks: z.array(scheduledTaskOutputSchema),
});

const taskBackendResponseSchema = z.strictObject({
  ok: z.literal(true),
  task: scheduledTaskOutputSchema,
});

const deleteBackendResponseSchema = z.strictObject({ ok: z.literal(true) });

const listToolOutputSchema = z.strictObject({
  status: outcomeStatusSchema,
  message: z.string(),
  tasks: z.array(scheduledTaskOutputSchema),
});

const taskToolOutputSchema = z.strictObject({
  status: outcomeStatusSchema,
  message: z.string(),
  task: scheduledTaskOutputSchema.nullable(),
});

const deleteToolOutputSchema = z.strictObject({
  status: outcomeStatusSchema,
  message: z.string(),
  taskId: z.string(),
});

function scheduleFailureMessage(
  operation: "list" | "create" | "update" | "delete",
): string {
  const object = operation === "list" ? "scheduled tasks" : "scheduled task";
  return `Failed to ${operation} ${object}: the Analytix schedule backend did not return a valid successful response.`;
}

export function createClawScheduleMcpServer(
  options: McpLaunchOptions,
): McpServer {
  const server = new McpServer(
    { name: "analytix-schedule", version: "0.1.0" },
    { capabilities: { logging: {} } },
  );

  const registerListTool = (name: string): void => {
    server.registerTool(
      name,
      {
        description: name.startsWith("claw_")
          ? "Legacy alias. List scheduled tasks managed by the currently running Analytix app."
          : "List scheduled tasks managed by the currently running Analytix app.",
        inputSchema: z.strictObject({}),
        outputSchema: listToolOutputSchema,
        annotations: {
          readOnlyHint: true,
          destructiveHint: false,
          idempotentHint: true,
          openWorldHint: false,
        },
      },
      async () => {
        try {
          const result = listBackendResponseSchema.parse(
            await postJson(options, "/schedule/internal/list", {}),
          );
          const tasks = result.tasks;
          const message = tasks.length
            ? `Found ${tasks.length} scheduled task(s).`
            : "No scheduled tasks are configured.";
          return textResult(message, { status: "success", message, tasks });
        } catch {
          const message = scheduleFailureMessage("list");
          return errorResult(message, {
            status: "failure",
            message,
            tasks: [],
          });
        }
      },
    );
  };
  registerListTool("claw_schedule_list");
  registerListTool("gui_schedule_list");

  const registerCreateTool = (name: string): void => {
    server.registerTool(
      name,
      {
        description: name.startsWith("claw_")
          ? "Legacy alias. Create a scheduled task in Analytix. Supports one-time (`at`), daily, or interval schedules."
          : "Create a scheduled task in Analytix. Supports one-time (`at`), daily, or interval schedules. When creating from an existing conversation, pass that conversation's provider_id, model, and reasoning_effort so the scheduled task keeps the same execution settings.",
        inputSchema: z.strictObject({
          title: z
            .string()
            .min(1)
            .describe("Short task title shown in the GUI"),
          prompt: z
            .string()
            .min(1)
            .describe(
              "The prompt/instruction the agent should run at schedule time",
            ),
          schedule: createScheduleInputSchema.describe(
            "Complete one-time, daily, or interval schedule",
          ),
          workspace_root: z
            .string()
            .optional()
            .describe("Optional workspace directory override"),
          claw_channel_id: z
            .string()
            .optional()
            .describe(
              "Optional Connect Phone channel id whose persona should run this task",
            ),
          provider_id: z
            .string()
            .optional()
            .describe(
              "Optional model provider id configured in Analytix settings",
            ),
          model: z
            .string()
            .optional()
            .describe(
              "Optional model id, e.g. deepseek-v4-pro / deepseek-v4-flash",
            ),
          reasoning_effort: z
            .enum(["auto", "off", "low", "medium", "high", "max"])
            .optional()
            .describe("Optional reasoning strength"),
          mode: z.enum(["agent", "plan"]).optional().describe("Execution mode"),
          enabled: z
            .boolean()
            .optional()
            .describe("Whether the task should be enabled immediately"),
        }),
        outputSchema: taskToolOutputSchema,
        annotations: {
          readOnlyHint: false,
          destructiveHint: false,
          idempotentHint: false,
          openWorldHint: false,
        },
      },
      async (args) => {
        try {
          const result = taskBackendResponseSchema.parse(
            await postJson(options, "/schedule/internal/create", {
              input: {
                title: args.title,
                prompt: args.prompt,
                workspaceRoot: args.workspace_root,
                clawChannelId: args.claw_channel_id,
                providerId: args.provider_id,
                model: args.model,
                reasoningEffort: args.reasoning_effort,
                mode: args.mode,
                enabled: args.enabled,
                schedule: args.schedule,
              },
            }),
          );
          const task = result.task;
          const message = `Scheduled task created: ${task.title}`;
          return textResult(message, { status: "success", message, task });
        } catch {
          const message = scheduleFailureMessage("create");
          return errorResult(message, {
            status: "failure",
            message,
            task: null,
          });
        }
      },
    );
  };
  registerCreateTool("claw_schedule_create");
  registerCreateTool("gui_schedule_create");

  const registerUpdateTool = (name: string): void => {
    server.registerTool(
      name,
      {
        description: name.startsWith("claw_")
          ? "Legacy alias. Update an existing Analytix scheduled task."
          : "Update an existing Analytix scheduled task.",
        inputSchema: z.strictObject({
          task_id: z
            .string()
            .min(1)
            .describe(
              "Task id returned by gui_schedule_list or gui_schedule_create",
            ),
          title: z.string().optional(),
          prompt: z.string().optional(),
          enabled: z.boolean().optional(),
          workspace_root: z.string().optional(),
          claw_channel_id: z.string().optional(),
          provider_id: z.string().optional(),
          model: z.string().optional(),
          reasoning_effort: z
            .enum(["auto", "off", "low", "medium", "high", "max"])
            .optional(),
          mode: z.enum(["agent", "plan"]).optional(),
          schedule: updateScheduleInputSchema.optional(),
        }),
        outputSchema: taskToolOutputSchema,
        annotations: {
          readOnlyHint: false,
          destructiveHint: false,
          idempotentHint: false,
          openWorldHint: false,
        },
      },
      async (args) => {
        try {
          const patch: Record<string, unknown> = {};
          if (args.title !== undefined) patch.title = args.title;
          if (args.prompt !== undefined) patch.prompt = args.prompt;
          if (args.enabled !== undefined) patch.enabled = args.enabled;
          if (args.workspace_root !== undefined)
            patch.workspaceRoot = args.workspace_root;
          if (args.claw_channel_id !== undefined)
            patch.clawChannelId = args.claw_channel_id;
          if (args.provider_id !== undefined)
            patch.providerId = args.provider_id;
          if (args.model !== undefined) patch.model = args.model;
          if (args.reasoning_effort !== undefined)
            patch.reasoningEffort = args.reasoning_effort;
          if (args.mode !== undefined) patch.mode = args.mode;
          if (args.schedule !== undefined) patch.schedule = args.schedule;
          const result = taskBackendResponseSchema.parse(
            await postJson(options, "/schedule/internal/update", {
              taskId: args.task_id,
              patch,
            }),
          );
          const task = result.task;
          const message = `Scheduled task updated: ${task.title}`;
          return textResult(message, { status: "success", message, task });
        } catch {
          const message = scheduleFailureMessage("update");
          return errorResult(message, {
            status: "failure",
            message,
            task: null,
          });
        }
      },
    );
  };
  registerUpdateTool("claw_schedule_update");
  registerUpdateTool("gui_schedule_update");

  const registerDeleteTool = (name: string): void => {
    server.registerTool(
      name,
      {
        description: name.startsWith("claw_")
          ? "Legacy alias. Delete a scheduled task from Analytix."
          : "Delete a scheduled task from Analytix.",
        inputSchema: z.strictObject({
          task_id: z
            .string()
            .min(1)
            .describe(
              "Task id returned by gui_schedule_list or gui_schedule_create",
            ),
        }),
        outputSchema: deleteToolOutputSchema,
        annotations: {
          readOnlyHint: false,
          destructiveHint: true,
          idempotentHint: false,
          openWorldHint: false,
        },
      },
      async ({ task_id }) => {
        try {
          deleteBackendResponseSchema.parse(
            await postJson(options, "/schedule/internal/delete", {
              taskId: task_id,
            }),
          );
          const message = `Scheduled task deleted: ${task_id}`;
          return textResult(message, {
            status: "success",
            message,
            taskId: task_id,
          });
        } catch {
          const message = scheduleFailureMessage("delete");
          return errorResult(message, {
            status: "failure",
            message,
            taskId: task_id,
          });
        }
      },
    );
  };
  registerDeleteTool("claw_schedule_delete");
  registerDeleteTool("gui_schedule_delete");

  // The `gui_plan_create` MCP tool has been retired in favour of the
  // native Analytix `create_plan` tool. See RETIRED_CLAW_GUI_PLAN_TOOL_NAMES
  // for the list of removed tool names.

  return server;
}

export async function runClawScheduleMcpServerFromArgv(
  argv: string[],
): Promise<boolean> {
  const options = parseLaunchOptions(argv);
  if (!options) return false;
  const server = createClawScheduleMcpServer(options);
  const transport = new StdioServerTransport();
  await server.connect(transport);
  return true;
}

/**
 * List of MCP tool names that used to act as the GUI plan bridge. The
 * names are kept here as a single source of truth for migration
 * scripts; the actual tools are no longer registered.
 */
export const RETIRED_CLAW_GUI_PLAN_TOOL_NAMES: readonly string[] = [
  "gui_plan_create",
] as const;
