#!/usr/bin/env node
"use strict";

globalThis.require = require;
globalThis.fs = require("fs");
globalThis.TextEncoder = require("util").TextEncoder;
globalThis.TextDecoder = require("util").TextDecoder;

globalThis.performance ??= require("performance");

globalThis.crypto ??= require("crypto");

require("./wasm_exec");

const path = require('path');

const go = new Go();
go.argv = process.argv.slice(1);
go.env = Object.assign({ TMPDIR: require("os").tmpdir() }, process.env);
go.exit = process.exit;
WebAssembly.instantiate(fs.readFileSync(path.join(__dirname, './supabase-cli.wasm')), go.importObject).then((result) => {
	process.on("exit", (code) => { // Node.js exits if no event handler is pending
		if (code === 0 && !go.exited) {
			// deadlock, make Go print error and stack traces
			go._pendingEvent = { id: 0 };
			go._resume();
		}
	});
	return go.run(result.instance);
}).catch((err) => {
	console.error(err);
	process.exit(1);
});
