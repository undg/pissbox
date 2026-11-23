import { CustomEditor } from "@earendil-works/pi-coding-agent";

export default function (pi) {
	pi.on("session_start", (_event, ctx) => {
		if (ctx.mode !== "tui") return;

		const previous = ctx.ui.getEditorComponent();
		ctx.ui.setEditorComponent((tui, theme, keybindings) => {
			const editor = previous?.(tui, theme, keybindings)
				?? new CustomEditor(tui, theme, keybindings, { embedWorkingStatus: true });
			const handleInput = editor.handleInput.bind(editor);
			editor.handleInput = (data) => {
				const submit = editor.onSubmit;
				// Pi trims before onSubmit. Match the untouched buffer, but let the editor decide whether to submit.
				if (!submit || (editor.getExpandedText?.() ?? editor.getText()) !== ":q") {
					handleInput(data);
					return;
				}
				editor.onSubmit = (text) => {
					if (text === ":q") ctx.shutdown();
					else submit.call(editor, text);
				};
				try {
					handleInput(data);
				} finally {
					editor.onSubmit = submit;
				}
			};
			return editor;
		});
	});
}
