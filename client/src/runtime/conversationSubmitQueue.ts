export interface ConversationSubmitQueue {
  enqueue<T>(task: () => Promise<T>): Promise<T>;
}

export function createConversationSubmitQueue(): ConversationSubmitQueue {
  let tail: Promise<unknown> = Promise.resolve();
  return {
    enqueue<T>(task: () => Promise<T>) {
      const result = tail.catch(() => undefined).then(task);
      tail = result.catch(() => undefined);
      return result;
    },
  };
}
