export function mapToLines(values?: Record<string, string>) {
  if (!values) return ''
  return Object.entries(values)
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}

export function linesToList(value?: string) {
  return (value ?? '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
}

export function linesToMap(value?: string) {
  const result: Record<string, string> = {}
  for (const line of linesToList(value)) {
    const index = line.indexOf('=')
    if (index <= 0) continue
    result[line.slice(0, index).trim()] = line.slice(index + 1)
  }
  return Object.keys(result).length > 0 ? result : undefined
}

