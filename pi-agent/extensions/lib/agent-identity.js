import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

const names = readFileSync(new URL("../agent-names.txt", import.meta.url), "utf8")
	.split(/\r?\n/)
	.map((name) => name.trim())
	.filter((name) => name && !name.startsWith("#"));
if (names.length !== 50 || new Set(names).size !== names.length ||
	names.some((name) => !/^[A-Za-z][A-Za-z0-9]{0,31}$/.test(name))) {
	throw new Error("agent-names.txt must contain exactly 50 unique, simple names");
}

/** @param {string} sessionId */
export function getAgentCallsign(sessionId) {
	if (typeof sessionId !== "string" || !sessionId) throw new Error("Pi session ID is required for an agent callsign");
	const digest = createHash("sha256").update(sessionId).digest();
	const first = digest.readUInt32BE(0) % names.length;
	let second = digest.readUInt32BE(4) % (names.length - 1);
	if (second >= first) second++;
	return `${names[first]} ${names[second]}`;
}
