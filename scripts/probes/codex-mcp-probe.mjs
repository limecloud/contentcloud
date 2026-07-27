#!/usr/bin/env node

import { appendFileSync } from "node:fs";
import { createInterface } from "node:readline";

const logPath = process.env.CONTENTCLOUD_MCP_PROBE_LOG;
if (!logPath) {
  process.stderr.write("CONTENTCLOUD_MCP_PROBE_LOG is required\n");
  process.exit(2);
}

let rootsRequested = false;

function log(event, data = {}) {
  appendFileSync(
    logPath,
    `${JSON.stringify({ at: new Date().toISOString(), event, ...data })}\n`,
    { encoding: "utf8", mode: 0o600 },
  );
}

function send(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

function clientSupportsRoots(params) {
  return Boolean(params?.capabilities?.roots);
}

function handleRequest(message) {
  switch (message.method) {
    case "initialize": {
      const params = message.params ?? {};
      log("initialize", {
        protocolVersion: params.protocolVersion ?? null,
        clientInfo: params.clientInfo ?? null,
        capabilities: params.capabilities ?? {},
      });
      send({
        jsonrpc: "2.0",
        id: message.id,
        result: {
          protocolVersion: params.protocolVersion ?? "2025-03-26",
          capabilities: {
            tools: { listChanged: false },
            resources: { subscribe: false, listChanged: false },
          },
          serverInfo: { name: "contentcloud-codex-probe", version: "1.0.0" },
          instructions:
            "ContentCloud Codex capability probe. Do not call the probe tool unless explicitly requested.",
        },
      });
      if (clientSupportsRoots(params) && !rootsRequested) {
        rootsRequested = true;
        send({ jsonrpc: "2.0", id: "contentcloud-roots-probe", method: "roots/list" });
        log("roots_list_requested");
      }
      return;
    }
    case "ping":
      send({ jsonrpc: "2.0", id: message.id, result: {} });
      return;
    case "tools/list":
      log("tools_list");
      send({
        jsonrpc: "2.0",
        id: message.id,
        result: {
          tools: [
            {
              name: "probe_status",
              description: "Return static MCP probe status",
              inputSchema: { type: "object", additionalProperties: false },
            },
          ],
        },
      });
      return;
    case "tools/call":
      log("tool_called", { name: message.params?.name ?? null });
      send({
        jsonrpc: "2.0",
        id: message.id,
        result: {
          content: [{ type: "text", text: "probe-ready" }],
          structuredContent: { status: "probe-ready" },
          isError: false,
        },
      });
      return;
    case "resources/list":
      log("resources_list");
      send({
        jsonrpc: "2.0",
        id: message.id,
        result: {
          resources: [
            {
              uri: "contentcloud-probe://status",
              name: "ContentCloud MCP probe status",
              mimeType: "application/json",
            },
          ],
        },
      });
      return;
    case "resources/read":
      log("resource_read", { uri: message.params?.uri ?? null });
      send({
        jsonrpc: "2.0",
        id: message.id,
        result: {
          contents: [
            {
              uri: "contentcloud-probe://status",
              mimeType: "application/json",
              text: '{"status":"probe-ready"}',
            },
          ],
        },
      });
      return;
    case "notifications/initialized":
    case "notifications/cancelled":
      log(message.method);
      return;
    default:
      log("unknown_method", { method: message.method ?? null });
      if (message.id !== undefined) {
        send({
          jsonrpc: "2.0",
          id: message.id,
          error: { code: -32601, message: "method not found" },
        });
      }
  }
}

function handleMessage(message) {
  if (message.method) {
    handleRequest(message);
    return;
  }
  if (message.id === "contentcloud-roots-probe") {
    if (message.error) {
      log("roots_list_error", { error: message.error });
    } else {
      const roots = Array.isArray(message.result?.roots) ? message.result.roots : [];
      log("roots_list_result", {
        roots: roots.map((root) => ({ uri: root.uri ?? null, name: root.name ?? null })),
      });
    }
  }
}

log("probe_started", { node: process.version, cwd: process.cwd() });

const lines = createInterface({ input: process.stdin, crlfDelay: Infinity });
lines.on("line", (line) => {
  try {
    handleMessage(JSON.parse(line));
  } catch (error) {
    log("parse_error", { message: error instanceof Error ? error.message : String(error) });
    send({ jsonrpc: "2.0", error: { code: -32700, message: "parse error" } });
  }
});
lines.on("close", () => log("probe_stopped"));
