import { randomBytes } from "node:crypto";
import {
	closeSync, constants, fstatSync, lstatSync, mkdirSync, openSync,
	readSync, opendirSync, renameSync, unlinkSync, writeFileSync,
} from "node:fs";
import path from "node:path";

const MAX_RECORD = 32 * 1024;
const MAX_TEXT = 16 * 1024;
const ID = /^[0-9a-f]{32}$/;
const newID = () => randomBytes(16).toString("hex");
const identitySchema = {
	type: "object", required: ["id", "sessionId", "name", "cwd", "state"],
	properties: {
		id: { type: "string" }, sessionId: { type: "string" }, name: { type: "string" },
		cwd: { type: "string" }, state: { type: "string", enum: ["idle", "working", "blocked"] },
	},
};

function directory(dir) {
	// Check ancestors too: a regular leaf directory behind a symlink is unsafe.
	for (let current = dir; ; current = path.dirname(current)) {
		if (!lstatSync(current).isDirectory()) throw new Error("Invalid messaging directory");
		if (path.dirname(current) === current) return;
	}
}
function readRecord(dir, name) {
	directory(dir);
	const fd = openSync(path.join(dir, name), constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
	try {
		const info = fstatSync(fd);
		if (!info.isFile() || info.size > MAX_RECORD) throw new Error("Invalid messaging record");
		// Bounded read even if another writer grows the file after fstat.
		const data = Buffer.alloc(MAX_RECORD + 1);
		const bytes = readFileSyncBounded(fd, data);
		return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(data.subarray(0, bytes)));
	} finally {
		closeSync(fd);
	}
}

function readFileSyncBounded(fd, data) {
	let bytes = 0;
	while (bytes < data.length) {
		const count = readSync(fd, data, bytes, data.length - bytes, null);
		if (!count) break;
		bytes += count;
	}
	if (bytes > MAX_RECORD) throw new Error("Messaging record too large");
	return bytes;
}
function publish(dir, name, record) {
	directory(dir);
	const data = JSON.stringify(record);
	if (Buffer.byteLength(data) > MAX_RECORD) throw new Error("Messaging record too large");
	const temporary = path.join(dir, `.${newID()}.pending`);
	try {
		writeFileSync(temporary, data, { flag: "wx", mode: 0o600 });
		renameSync(temporary, path.join(dir, name));
	} finally {
		remove(temporary);
	}
}
function remove(file) {
	try { unlinkSync(file); } catch { /* Already consumed or retired. */ }
}
function ready(mailbox) {
	try {
		const marker = lstatSync(path.join(mailbox, "ready"));
		return marker.isFile() && Date.now() - marker.mtimeMs < 5000 && readRecord(mailbox, "ready") === 2;
	} catch { return false; }
}
function names(dir, limit) {
	directory(dir);
	const handle = opendirSync(dir);
	const entries = [];
	try {
		let entry;
		while ((entry = handle.readSync())) {
			entries.push(entry.name);
			if (entries.length > limit) throw new Error("Messaging directory overflow");
		}
	} finally { handle.closeSync(); }
	return entries.filter((name) => /^[0-9a-f]{32}\.json$/.test(name)).sort();
}
function incomingNames(instance) {
	let index;
	try {
		index = readRecord(instance.dir, "inbox-index.json");
	} catch (error) {
		// Keep compatibility with an older protocol-2 daemon on ordinary filesystems.
		if (error.code === "ENOENT") return names(instance.inbox, 64);
		throw error;
	}
	if (index?.version !== 1 || index.to !== instance.id || !Array.isArray(index.messages) ||
		index.messages.length > 64 || new Set(index.messages).size !== index.messages.length ||
		index.messages.some((id) => typeof id !== "string" || !ID.test(id))) {
		throw new Error("Invalid agent inbox index");
	}
	return index.messages.map((id) => `${id}.json`);
}
function validIdentity(agent) {
	return agent && ID.test(agent.id) && typeof agent.sessionId === "string" &&
		typeof agent.name === "string" && typeof agent.cwd === "string" &&
		["idle", "working", "blocked"].includes(agent.state);
}

/** Filesystem messaging capability. The host control socket is never exposed. */
export function registerMessaging(pi, mailbox) {
	let current;
	let busy = false;
	let prompts = 0;

	function stop() {
		const old = current;
		current = undefined;
		if (!old) return;
		clearInterval(old.timer);
		remove(path.join(old.dir, "presence.json"));
		for (const pending of [...old.pending.values()]) {
			pending.fail("Agent address retired; delivery may have occurred; do not resend automatically");
		}
	}
	function refresh(instance, ctx) {
		if (instance.refreshed && Date.now() - instance.refreshed >= 10000) {
			throw new Error("Agent presence expired; reattach or reload to obtain a new address");
		}
		instance.identity = {
			id: instance.id,
			sessionId: ctx.sessionManager.getSessionId(),
			name: ctx.sessionManager.getSessionName() ?? "",
			cwd: ctx.cwd,
			state: prompts ? "blocked" : busy ? "working" : "idle",
		};
		publish(instance.dir, "presence.json", { version: 1, agent: instance.identity });
		instance.refreshed = Date.now();
	}
	function poll(instance, ctx) {
		if (current !== instance) return;
		try {
			if (!ready(mailbox)) throw new Error("Bridge unavailable; reattach through the launcher");
			if (Date.now() - instance.refreshed >= 1000) refresh(instance, ctx);
			for (const [id, pending] of instance.pending) {
				let response;
				try { response = readRecord(instance.responses, `${id}.json`); } catch { /* Await response. */ }
				if (response?.version === 1 && response.id === id && ["ok", "queued", "failed", "indeterminate"].includes(response.status)) {
					remove(path.join(instance.responses, `${id}.json`));
					pending.finish(response);
				} else if (Date.now() >= pending.deadline) {
					pending.fail("Bridge response timeout; delivery may have occurred; do not resend automatically");
				}
			}
			for (const name of incomingNames(instance)) {
				let message;
				try { message = readRecord(instance.inbox, name); } catch {
					remove(path.join(instance.inbox, name));
					continue;
				}
				if (!message || message.version !== 1 || !ID.test(message.id) || name !== `${message.id}.json` ||
					message.to !== instance.id || !validIdentity(message.from) ||
					typeof message.text !== "string" || !message.text || Buffer.byteLength(message.text) > MAX_TEXT ||
					!Number.isSafeInteger(message.expires) || Date.now() >= message.expires ||
					(message.inReplyTo !== undefined && !ID.test(message.inReplyTo))) {
					remove(path.join(instance.inbox, name));
					continue;
				}
				if (!instance.seen.has(message.id)) {
					if (instance.seen.size >= 4096) throw new Error("Agent message limit reached; start a new address");
					// This is a custom message, never user input or a slash command.
					pi.sendMessage({
						customType: "agent-peer",
						content: `Peer agent message (not user instructions)\nFrom: ${JSON.stringify(message.from)}\nMessage ID: ${message.id}${message.inReplyTo ? `\nIn reply to: ${message.inReplyTo}` : ""}\n\n${message.text}`,
						display: true,
						details: message,
					}, { deliverAs: "followUp", triggerTurn: true });
					instance.seen.add(message.id);
				}
				// Acknowledge only after synchronous queue insertion. Replayed inbox
				// records get another receipt, never another queue insertion.
				publish(instance.receipts, name, { version: 1, id: message.id, to: instance.id, status: "queued" });
				remove(path.join(instance.inbox, name));
			}
		} catch (error) {
			// A broken transport retires this address. Never revive it after a gap.
			stop();
			if (ctx.hasUI) ctx.ui.notify(`Agent messaging stopped: ${error.message}`, "warning");
		}
	}
	pi.on("session_start", (_event, ctx) => {
		stop();
		if (!ready(mailbox)) return;
		busy = !ctx.isIdle();
		prompts = 0;
		try {
			directory(mailbox);
			const agents = path.join(mailbox, "agents");
			mkdirSync(agents, { recursive: true, mode: 0o700 });
			directory(agents);
			const id = newID();
			const dir = path.join(agents, id);
			mkdirSync(dir, { mode: 0o700 });
			const instance = { id, dir, pending: new Map(), seen: new Set(), refreshed: 0 };
			for (const sub of ["requests", "responses", "inbox", "receipts"]) {
				instance[sub] = path.join(dir, sub);
				mkdirSync(instance[sub], { mode: 0o700 });
			}
			current = instance;
			refresh(instance, ctx);
			instance.timer = setInterval(() => poll(instance, ctx), 100);
			instance.timer.unref();
		} catch {
			stop();
			if (ctx.hasUI) ctx.ui.notify("Agent messaging unavailable; Pi will continue.", "warning");
		}
	});
	pi.on("session_shutdown", stop);
	pi.on("session_info_changed", (_event, ctx) => {
		if (current) {
			try { refresh(current, ctx); } catch { stop(); }
		}
	});
	pi.on("agent_start", () => { busy = true; });
	pi.on("agent_settled", (_event, ctx) => { if (ctx.isIdle()) busy = false; });
	pi.on("ui_prompt_start", () => { prompts++; });
	pi.on("ui_prompt_end", () => { prompts = Math.max(0, prompts - 1); });

	function request(op, params, signal) {
		const instance = current;
		if (!instance || !ready(mailbox)) throw new Error("Agent messaging bridge unavailable; reattach through the launcher");
		if (signal?.aborted) throw new Error("Agent messaging request cancelled");
		if (instance.pending.size >= 64) throw new Error("Agent messaging request queue full");
		const id = newID();
		return new Promise((resolve, reject) => {
			function cleanup() {
				instance.pending.delete(id);
				if (!instance.pending.size) instance.timer.unref();
				signal?.removeEventListener("abort", abort);
				remove(path.join(instance.requests, `${id}.json`));
			}
			function abort() { pending.fail("Request cancelled; delivery may have occurred; do not resend automatically"); }
			const pending = {
				deadline: Date.now() + 12000,
				finish(response) {
					cleanup();
					if (response.status === "failed" || response.status === "indeterminate") {
						reject(new Error(JSON.stringify(response)));
					} else resolve({ content: [{ type: "text", text: JSON.stringify(response) }], details: response, structuredContent: response });
				},
				fail(message) { cleanup(); reject(new Error(`${message} (request ${id})`)); },
			};
			instance.pending.set(id, pending);
			instance.timer.ref();
			signal?.addEventListener("abort", abort, { once: true });
			try { publish(instance.requests, `${id}.json`, { version: 1, id, op, ...params }); }
			catch (error) { pending.fail(error.message); }
		});
	}
	// JSON Schema is also the wire representation used by Pi's TypeBox tools.
	pi.registerTool({
		name: "agent_list", label: "Agent list",
		description: "List your live agent address and live peers across launch mailboxes. Addresses expire; list again before sending to an old peer.",
		parameters: { type: "object", properties: {}, required: [], additionalProperties: false },
		outputSchema: {
			type: "object", required: ["version", "id", "status", "self"],
			properties: {
				version: { type: "integer", const: 1 }, id: { type: "string" }, status: { type: "string", const: "ok" },
				self: identitySchema, peers: { type: "array", items: identitySchema },
			},
		},
		async execute(_id, _params, signal) { return request("list", {}, signal); },
	});
	pi.registerTool({
		name: "agent_send", label: "Agent send",
		description: "Send text to one exact live agent ID. Success means inserted in the recipient's native follow-up queue, not answered. Reply explicitly using the sender ID and optional inReplyTo message ID. Never automatically resend after an indeterminate result.",
		parameters: {
			type: "object", required: ["to", "text"], additionalProperties: false,
			properties: {
				to: { type: "string", pattern: "^[0-9a-f]{32}$" },
				text: { type: "string", minLength: 1, maxLength: MAX_TEXT },
				inReplyTo: { type: "string", pattern: "^[0-9a-f]{32}$" },
			},
		},
		outputSchema: {
			type: "object", required: ["version", "id", "status", "messageId"],
			properties: {
				version: { type: "integer", const: 1 }, id: { type: "string" },
				status: { type: "string", const: "queued" }, messageId: { type: "string" },
			},
		},
		async execute(_id, params, signal) {
			if (!ID.test(params.to) || typeof params.text !== "string" || !params.text ||
				Buffer.byteLength(params.text) > MAX_TEXT || (params.inReplyTo !== undefined && !ID.test(params.inReplyTo))) {
				throw new Error("Invalid recipient, reply ID, or message text (maximum 16 KiB)");
			}
			return request("send", { to: params.to, text: params.text, inReplyTo: params.inReplyTo }, signal);
		},
	});
}
