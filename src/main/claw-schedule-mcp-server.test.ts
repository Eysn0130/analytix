import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createClawScheduleMcpServer,
  RETIRED_CLAW_GUI_PLAN_TOOL_NAMES,
} from "./claw-schedule-mcp-server";

const activeToolNames = [
  "claw_schedule_create",
  "claw_schedule_delete",
  "claw_schedule_list",
  "claw_schedule_update",
  "gui_schedule_create",
  "gui_schedule_delete",
  "gui_schedule_list",
  "gui_schedule_update",
] as const;

const scheduledTask = {
  id: "task-1",
  title: "Daily review",
  enabled: true,
  prompt: "Review evidence",
  workspaceRoot: "/workspace",
  clawChannelId: "",
  providerId: "provider-1",
  model: "model-1",
  reasoningEffort: "off",
  mode: "agent",
  schedule: { kind: "daily", everyMinutes: 0, timeOfDay: "09:00", atTime: "" },
  createdAt: "2026-07-11T00:00:00Z",
  updatedAt: "2026-07-11T00:00:00Z",
  lastRunAt: "",
  nextRunAt: "2026-07-12T01:00:00Z",
  lastStatus: "idle",
  lastMessage: "",
  lastThreadId: "",
} as const;

const validArguments: Record<
  (typeof activeToolNames)[number],
  Record<string, unknown>
> = {
  claw_schedule_create: {
    title: "Daily review",
    prompt: "Review evidence",
    schedule: { kind: "daily", timeOfDay: "09:00" },
  },
  gui_schedule_create: {
    title: "Daily review",
    prompt: "Review evidence",
    schedule: { kind: "interval", everyMinutes: 30 },
  },
  claw_schedule_delete: { task_id: "task-1" },
  gui_schedule_delete: { task_id: "task-1" },
  claw_schedule_list: {},
  gui_schedule_list: {},
  claw_schedule_update: { task_id: "task-1", title: "Updated" },
  gui_schedule_update: {
    task_id: "task-1",
    schedule: { kind: "at", atTime: "2026-07-12T01:00:00+08:00" },
  },
};

async function connectScheduleMcp() {
  const server = createClawScheduleMcpServer({
    baseUrl: "http://127.0.0.1:8787",
    secret: "test-secret",
  });
  const client = new Client({
    name: "schedule-contract-test",
    version: "1.0.0",
  });
  const [clientTransport, serverTransport] =
    InMemoryTransport.createLinkedPair();
  await server.connect(serverTransport);
  await client.connect(clientTransport);
  return {
    client,
    close: async (): Promise<void> => {
      await client.close();
      await server.close();
    },
  };
}

function expectRecursivelyClosedSchema(value: unknown, path = "$"): void {
  if (!value || typeof value !== "object" || Array.isArray(value)) return;
  const schema = value as Record<string, unknown>;
  const types = Array.isArray(schema.type) ? schema.type : [schema.type];
  if (types.includes("object")) {
    expect(schema.additionalProperties, `${path}.additionalProperties`).toBe(
      false,
    );
  }
  const properties = schema.properties;
  if (
    properties &&
    typeof properties === "object" &&
    !Array.isArray(properties)
  ) {
    for (const [name, child] of Object.entries(properties)) {
      expectRecursivelyClosedSchema(child, `${path}.properties.${name}`);
    }
  }
  if (schema.items)
    expectRecursivelyClosedSchema(schema.items, `${path}.items`);
  for (const keyword of ["anyOf", "oneOf", "allOf"] as const) {
    const branches = schema[keyword];
    if (!Array.isArray(branches)) continue;
    branches.forEach((branch, index) =>
      expectRecursivelyClosedSchema(branch, `${path}.${keyword}[${index}]`),
    );
  }
}

function successResponseFor(url: string): Record<string, unknown> {
  if (url.endsWith("/list")) return { ok: true, tasks: [scheduledTask] };
  if (url.endsWith("/delete")) return { ok: true };
  return { ok: true, task: scheduledTask };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("claw schedule MCP server: GUI plan bridge retirement", () => {
  it("records gui_plan_create as a retired tool name", () => {
    expect(RETIRED_CLAW_GUI_PLAN_TOOL_NAMES).toContain("gui_plan_create");
  });

  it("no longer exposes the legacy tool name as a registered export", async () => {
    // The legacy tool was previously exported via the module surface
    // and registered through `server.registerTool`. The retirement
    // keeps the retired name in the readonly list and removes the
    // registration; this regression check ensures the constant list
    // exists for migration scripts and does not include any active
    // tool names.
    expect(RETIRED_CLAW_GUI_PLAN_TOOL_NAMES.length).toBeGreaterThan(0);
    for (const name of RETIRED_CLAW_GUI_PLAN_TOOL_NAMES) {
      expect(name).toBe("gui_plan_create");
    }
    const moduleExports = await import("./claw-schedule-mcp-server");
    expect(
      (moduleExports as { registerTool?: unknown }).registerTool,
    ).toBeUndefined();
  });
});

describe("claw schedule MCP server: strict bidirectional contract", () => {
  it("advertises exactly eight recursively closed input and output schemas", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const connection = await connectScheduleMcp();
    try {
      const catalog = await connection.client.listTools();
      expect(catalog.tools.map((tool) => tool.name).sort()).toEqual(
        [...activeToolNames].sort(),
      );
      for (const tool of catalog.tools) {
        expect(tool.inputSchema.$schema).toMatch(
          /^https?:\/\/json-schema\.org\//,
        );
        expect(tool.outputSchema?.$schema).toMatch(
          /^https?:\/\/json-schema\.org\//,
        );
        expectRecursivelyClosedSchema(
          tool.inputSchema,
          `${tool.name}.inputSchema`,
        );
        expectRecursivelyClosedSchema(
          tool.outputSchema,
          `${tool.name}.outputSchema`,
        );
      }
      const listTool = catalog.tools.find(
        (tool) => tool.name === "gui_schedule_list",
      );
      expect(listTool?.annotations?.readOnlyHint).toBe(true);
    } finally {
      await connection.close();
    }
  });

  it("returns schema-shaped structured content for every tool success", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const url = String(input);
        return new Response(JSON.stringify(successResponseFor(url)), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    const connection = await connectScheduleMcp();
    try {
      await connection.client.listTools();
      for (const name of activeToolNames) {
        const result = await connection.client.callTool({
          name,
          arguments: validArguments[name],
        });
        expect(result.isError, name).not.toBe(true);
        expect(
          (result.structuredContent as { status?: string } | undefined)?.status,
          name,
        ).toBe("success");
      }
    } finally {
      await connection.close();
    }
  });

  it("never upgrades malformed or ok-false HTTP responses to success", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              ok: false,
              message: "account 6217000012345678901 must not be reflected",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );
    const connection = await connectScheduleMcp();
    try {
      await connection.client.listTools();
      for (const name of activeToolNames) {
        const result = await connection.client.callTool({
          name,
          arguments: validArguments[name],
        });
        expect(result.isError, name).toBe(true);
        const structured = result.structuredContent as
          { status?: string; message?: string } | undefined;
        expect(structured?.status, name).toBe("failure");
        expect(structured?.message, name).not.toContain("6217000012345678901");
      }
    } finally {
      await connection.close();
    }
  });
});
