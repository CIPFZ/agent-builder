export interface ToolOutputPreview {
  text: string;
  truncated: boolean;
}

export function boundedToolText(value: string, maxLines = 24, maxChars = 6000): ToolOutputPreview {
  const normalized = value.trim();
  const lines = normalized.split(/\r?\n/);
  let text = lines.slice(0, maxLines).join('\n');
  let truncated = lines.length > maxLines;
  if (text.length > maxChars) {
    text = text.slice(0, maxChars);
    truncated = true;
  }
  return { text: truncated ? `${text}\n…` : text, truncated };
}
