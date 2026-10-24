import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { getAgentCallsign } from "./agent-identity.js";

const names = readFileSync(new URL("../agent-names.txt", import.meta.url), "utf8")
	.split(/\r?\n/).map((name) => name.trim()).filter(Boolean);

test("uses the editable pool of 50 names for stable, distinguishable session callsigns", () => {
	assert.equal(names.length, 50);
	assert.equal(new Set(names).size, 50);
	const first = getAgentCallsign("session-one");
	assert.equal(getAgentCallsign("session-one"), first);
	assert.notEqual(getAgentCallsign("session-two"), first);
	const parts = first.split(" ");
	assert.equal(parts.length, 2);
	assert.ok(names.includes(parts[0]));
	assert.ok(names.includes(parts[1]));
	assert.notEqual(parts[0], parts[1]);
	assert.match(first, /^[A-Za-z][A-Za-z0-9]* [A-Za-z][A-Za-z0-9]*$/);
	assert.throws(() => getAgentCallsign(""), /session ID is required/);
});
