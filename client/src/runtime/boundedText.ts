export interface BoundedText {
  text: string;
  truncated: boolean;
}

export function boundedText(value: string, maxLines = 24, maxChars = 3000): BoundedText {
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
