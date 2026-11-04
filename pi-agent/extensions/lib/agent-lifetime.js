// A separate process observes the lifetime pipe, not Pi's event loop. EOF means
// the owning Pi process exited (including SIGKILL), or explicitly retired this address.
import { lstatSync, renameSync, unlinkSync, writeFileSync } from "node:fs";
import path from "node:path";

const [dir, id] = process.argv.slice(2);
if (!dir || !path.isAbsolute(dir) || !/^[0-9a-f]{32}$/.test(id) || path.basename(dir) !== id) process.exit(1);
process.stdin.resume();
process.stdin.once("end", () => {
	try {
		for (let current = dir; ; current = path.dirname(current)) {
			if (!lstatSync(current).isDirectory()) return;
			if (path.dirname(current) === current) break;
		}
		const temporary = path.join(dir, `.closed-${process.pid}.pending`);
		writeFileSync(temporary, JSON.stringify({ version: 1, id }), { flag: "wx", mode: 0o600 });
		renameSync(temporary, path.join(dir, "closed.json"));
		try { unlinkSync(path.join(dir, "presence.json")); } catch { /* Already retired. */ }
	} catch { /* A missing mailbox needs no cleanup; never follow replacement links. */ }
});
