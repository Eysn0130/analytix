import { TextDecoder } from "node:util";

export const JSONRPC_STDIO_RUNTIME_VERSION = "0.15.0";

export const JSONRPC_STDIO_LIMITS = Object.freeze({
  maxFrameBytes: 4 * 1024 * 1024,
  maxStringBytes: 1024 * 1024,
  maxJsonTokens: 200_000,
  maxJsonDepth: 128,
  maxConcurrentRequests: 8,
  maxQueuedRequests: 32,
  maxOutstandingRequestBytes: 8 * 1024 * 1024,
  maxErrorDataBytes: 16 * 1024
});

const allowedRequestKeys = new Set(["jsonrpc", "id", "method", "params"]);
const utf8Decoder = new TextDecoder("utf-8", { fatal: true });

function requestIdKey(id) {
  if (typeof id === "string") return `s:${id}`;
  if (Number.isSafeInteger(id)) return `n:${id}`;
  return "";
}

function safeErrorData(value, maxBytes) {
  if (value === undefined) return undefined;
  try {
    const encoded = JSON.stringify(value);
    if (Buffer.byteLength(encoded, "utf8") > maxBytes) return undefined;
    return JSON.parse(encoded);
  } catch {
    return undefined;
  }
}

function jsonRpcError(id, code, message, data, maxErrorDataBytes) {
  const error = { code, message };
  const safeData = safeErrorData(data, maxErrorDataBytes);
  if (safeData !== undefined) error.data = safeData;
  return { jsonrpc: "2.0", id, error };
}

function scanStrictJson(source, limits) {
  let offset = 0;
  let tokens = 0;

  function token() {
    tokens += 1;
    if (tokens > limits.maxJsonTokens) throw new Error("JSON token limit exceeded");
  }

  function whitespace() {
    while (offset < source.length && (source[offset] === " " || source[offset] === "\t" || source[offset] === "\r" || source[offset] === "\n")) offset += 1;
  }

  function stringValue() {
    token();
    if (source[offset] !== '"') throw new Error("JSON string expected");
    const start = offset;
    offset += 1;
    while (offset < source.length) {
      const code = source.charCodeAt(offset);
      if (code === 0x22) {
        offset += 1;
        const value = JSON.parse(source.slice(start, offset));
        if (Buffer.byteLength(value, "utf8") > limits.maxStringBytes) throw new Error("JSON string limit exceeded");
        return value;
      }
      if (code < 0x20) throw new Error("JSON string contains a control character");
      if (code === 0x5c) {
        offset += 1;
        if (offset >= source.length) throw new Error("JSON string escape is incomplete");
        if (source[offset] === "u") {
          if (offset + 4 >= source.length) throw new Error("JSON unicode escape is incomplete");
          for (let index = offset + 1; index <= offset + 4; index += 1) {
            const digit = source.charCodeAt(index);
            const hex = (digit >= 0x30 && digit <= 0x39) || (digit >= 0x41 && digit <= 0x46) || (digit >= 0x61 && digit <= 0x66);
            if (!hex) throw new Error("JSON unicode escape is invalid");
          }
          offset += 4;
        } else if (!'"\\/bfnrt'.includes(source[offset])) {
          throw new Error("JSON string escape is invalid");
        }
      }
      offset += 1;
    }
    throw new Error("JSON string is incomplete");
  }

  function numberValue() {
    token();
    const start = offset;
    if (source[offset] === "-") offset += 1;
    if (source[offset] === "0") {
      offset += 1;
    } else {
      if (source[offset] < "1" || source[offset] > "9") throw new Error("JSON number is invalid");
      while (source[offset] >= "0" && source[offset] <= "9") offset += 1;
    }
    if (source[offset] === ".") {
      offset += 1;
      if (source[offset] < "0" || source[offset] > "9") throw new Error("JSON fraction is invalid");
      while (source[offset] >= "0" && source[offset] <= "9") offset += 1;
    }
    if (source[offset] === "e" || source[offset] === "E") {
      offset += 1;
      if (source[offset] === "+" || source[offset] === "-") offset += 1;
      if (source[offset] < "0" || source[offset] > "9") throw new Error("JSON exponent is invalid");
      while (source[offset] >= "0" && source[offset] <= "9") offset += 1;
    }
    if (!Number.isFinite(Number(source.slice(start, offset)))) throw new Error("JSON number is not finite");
  }

  function literal(expected) {
    token();
    if (source.slice(offset, offset + expected.length) !== expected) throw new Error("JSON literal is invalid");
    offset += expected.length;
  }

  function value(depth) {
    if (depth > limits.maxJsonDepth) throw new Error("JSON nesting limit exceeded");
    whitespace();
    const character = source[offset];
    if (character === "{") return objectValue(depth + 1);
    if (character === "[") return arrayValue(depth + 1);
    if (character === '"') return stringValue();
    if (character === "t") return literal("true");
    if (character === "f") return literal("false");
    if (character === "n") return literal("null");
    return numberValue();
  }

  function objectValue(depth) {
    token();
    offset += 1;
    whitespace();
    const keys = new Set();
    if (source[offset] === "}") {
      offset += 1;
      return;
    }
    while (offset < source.length) {
      whitespace();
      const key = stringValue();
      if (keys.has(key)) throw new Error("duplicate JSON object key");
      keys.add(key);
      whitespace();
      if (source[offset] !== ":") throw new Error("JSON object colon is missing");
      offset += 1;
      value(depth);
      whitespace();
      if (source[offset] === "}") {
        offset += 1;
        return;
      }
      if (source[offset] !== ",") throw new Error("JSON object separator is invalid");
      offset += 1;
    }
    throw new Error("JSON object is incomplete");
  }

  function arrayValue(depth) {
    token();
    offset += 1;
    whitespace();
    if (source[offset] === "]") {
      offset += 1;
      return;
    }
    while (offset < source.length) {
      value(depth);
      whitespace();
      if (source[offset] === "]") {
        offset += 1;
        return;
      }
      if (source[offset] !== ",") throw new Error("JSON array separator is invalid");
      offset += 1;
    }
    throw new Error("JSON array is incomplete");
  }

  whitespace();
  value(0);
  whitespace();
  if (offset !== source.length) throw new Error("JSON contains trailing content");
}

export function parseStrictJsonRpcEnvelope(raw, limits = JSONRPC_STDIO_LIMITS) {
  if (!Buffer.isBuffer(raw)) raw = Buffer.from(raw);
  if (!raw.length || raw.length > limits.maxFrameBytes) throw new Error("JSON-RPC frame size is invalid");
  let source;
  try {
    source = utf8Decoder.decode(raw);
  } catch {
    const error = new Error("JSON-RPC frame is not valid UTF-8");
    error.jsonRpcCode = -32700;
    throw error;
  }
  let message;
  try {
    message = JSON.parse(source);
  } catch {
    const error = new Error("JSON-RPC frame is not valid JSON");
    error.jsonRpcCode = -32700;
    throw error;
  }
  scanStrictJson(source, limits);
  if (!message || typeof message !== "object" || Array.isArray(message)) throw new Error("JSON-RPC message must be an object");
  if (Object.keys(message).some((key) => !allowedRequestKeys.has(key)) || message.jsonrpc !== "2.0") {
    throw new Error("JSON-RPC request envelope is invalid");
  }
  if (typeof message.method !== "string" || !message.method || message.method.trim() !== message.method) {
    throw new Error("JSON-RPC method is invalid");
  }
  if (message.params !== undefined && (!message.params || typeof message.params !== "object" || Array.isArray(message.params))) {
    throw new Error("JSON-RPC params must be an object");
  }
  if (Object.hasOwn(message, "id") && !requestIdKey(message.id)) {
    throw new Error("JSON-RPC request id is invalid");
  }
  return message;
}

export function startJsonRpcStdioRuntime({
  handleRequest,
  handleNotification = async () => {},
  close: closeHandler = async () => {},
  input = process.stdin,
  output = process.stdout,
  limits: limitOverrides = {}
}) {
  if (typeof handleRequest !== "function" || typeof handleNotification !== "function") {
    throw new Error("JSON-RPC stdio handlers are required");
  }
  const limits = Object.freeze({ ...JSONRPC_STDIO_LIMITS, ...limitOverrides });
  let buffer = Buffer.alloc(0);
  let discardingOversizedFrame = false;
  let closed = false;
  let inputEnded = false;
  let activeCount = 0;
  let outstandingBytes = 0;
  let writeChain = Promise.resolve();
  const queue = [];
  const inFlight = new Map();

  function writeMessage(message) {
    if (closed) return Promise.resolve();
    let encoded = Buffer.from(`${JSON.stringify(message)}\n`, "utf8");
    if (encoded.length > limits.maxFrameBytes) {
      encoded = Buffer.from(`${JSON.stringify(jsonRpcError(message?.id ?? null, -32603, "Internal error", undefined, limits.maxErrorDataBytes))}\n`, "utf8");
    }
    writeChain = writeChain.then(() => new Promise((resolve, reject) => {
      try {
        if (output.write(encoded)) resolve();
        else output.once("drain", resolve);
      } catch (error) {
        reject(error);
      }
    })).catch(() => {
      void shutdown();
    });
    return writeChain;
  }

  function cancelRequest(id, reason = "cancelled") {
    const key = requestIdKey(id);
    const record = key ? inFlight.get(key) : null;
    if (!record || record.message.method === "initialize") return false;
    record.cancelled = true;
    record.controller.abort(new Error(String(reason || "cancelled")));
    if (!record.active) {
      const position = queue.indexOf(record);
      if (position >= 0) queue.splice(position, 1);
      if (record.accounted) {
        outstandingBytes -= record.frameBytes;
        record.accounted = false;
      }
      if (inFlight.get(key) === record) inFlight.delete(key);
      pump();
    }
    return true;
  }

  function pump() {
    while (!closed && activeCount < limits.maxConcurrentRequests && queue.length) {
      const record = queue.shift();
      if (record.cancelled) {
        if (record.accounted) {
          outstandingBytes -= record.frameBytes;
          record.accounted = false;
        }
        continue;
      }
      activeCount += 1;
      record.active = true;
      void (async () => {
        try {
          const result = await handleRequest(record.message, { signal: record.controller.signal, requestId: record.message.id });
          if (!record.cancelled && result !== undefined) {
            await writeMessage({ jsonrpc: "2.0", id: record.message.id, result });
          }
        } catch (error) {
          if (!record.cancelled) {
            const code = Number.isInteger(error?.code) ? error.code : -32603;
            const message = code === -32603 ? "Internal error" : String(error?.message || "Request failed").slice(0, 1024);
            await writeMessage(jsonRpcError(record.message.id, code, message, code === -32603 ? undefined : error?.payload, limits.maxErrorDataBytes));
          }
        } finally {
          activeCount -= 1;
          if (record.accounted) {
            outstandingBytes -= record.frameBytes;
            record.accounted = false;
          }
          if (inFlight.get(record.key) === record) inFlight.delete(record.key);
          pump();
          if (inputEnded && activeCount === 0 && queue.length === 0) void finishGracefully();
        }
      })();
    }
  }

  function dispatchFrame(frame) {
    if (!frame.length) return;
    let message;
    try {
      message = parseStrictJsonRpcEnvelope(frame, limits);
    } catch (error) {
      const code = error?.jsonRpcCode === -32700 ? -32700 : -32600;
      void writeMessage(jsonRpcError(null, code, code === -32700 ? "Parse error" : "Invalid Request", undefined, limits.maxErrorDataBytes));
      return;
    }
    if (!Object.hasOwn(message, "id")) {
      void Promise.resolve(handleNotification(message, { cancelRequest })).catch(() => {});
      return;
    }
    const key = requestIdKey(message.id);
    const duplicate = inFlight.get(key);
    if (duplicate) {
      void writeMessage(jsonRpcError(message.id, -32600, "Invalid Request", undefined, limits.maxErrorDataBytes));
      return;
    }
    if (queue.length >= limits.maxQueuedRequests || outstandingBytes+frame.length > limits.maxOutstandingRequestBytes) {
      void writeMessage(jsonRpcError(message.id, -32603, "Server is busy", undefined, limits.maxErrorDataBytes));
      return;
    }
    const record = { key, message, frameBytes: frame.length, controller: new AbortController(), active: false, cancelled: false, accounted: true };
    inFlight.set(key, record);
    outstandingBytes += frame.length;
    queue.push(record);
    pump();
  }

  function consume(chunk) {
    if (closed || inputEnded) return;
    let incoming = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
    if (discardingOversizedFrame) {
      const newline = incoming.indexOf(0x0a);
      if (newline < 0) return;
      incoming = incoming.subarray(newline + 1);
      discardingOversizedFrame = false;
    }
    buffer = buffer.length ? Buffer.concat([buffer, incoming]) : Buffer.from(incoming);
    let newline = buffer.indexOf(0x0a);
    while (newline >= 0) {
      let frame = buffer.subarray(0, newline);
      buffer = buffer.subarray(newline + 1);
      if (frame.length && frame[frame.length - 1] === 0x0d) frame = frame.subarray(0, frame.length - 1);
      if (frame.length > limits.maxFrameBytes) {
        void writeMessage(jsonRpcError(null, -32600, "Invalid Request", undefined, limits.maxErrorDataBytes));
      } else {
        dispatchFrame(frame);
      }
      newline = buffer.indexOf(0x0a);
    }
    if (buffer.length > limits.maxFrameBytes) {
      buffer = Buffer.alloc(0);
      discardingOversizedFrame = true;
      void writeMessage(jsonRpcError(null, -32600, "Invalid Request", undefined, limits.maxErrorDataBytes));
    }
  }

  async function shutdown() {
    if (closed) return;
    closed = true;
    input.off("data", consume);
    for (const record of inFlight.values()) {
      record.cancelled = true;
      record.controller.abort(new Error("stdio connection closed"));
    }
    inFlight.clear();
    queue.length = 0;
    outstandingBytes = 0;
    try {
      await closeHandler();
    } catch {
      // Shutdown is already fail-closed; never write exception details to stdout.
    }
  }

  async function finishGracefully() {
    if (closed || activeCount !== 0 || queue.length !== 0) return;
    closed = true;
    input.off("data", consume);
    try {
      await closeHandler();
    } catch {
      // EOF has already removed request authority; no error is written.
    }
  }

  input.on("data", consume);
  input.once("end", () => {
    inputEnded = true;
    if (buffer.length && !discardingOversizedFrame) dispatchFrame(buffer);
    buffer = Buffer.alloc(0);
    if (activeCount === 0 && queue.length === 0) void finishGracefully();
  });
  input.once("error", () => void shutdown());

  return Object.freeze({ close: shutdown, cancelRequest, limits });
}
