export function getErrorMessage(err: unknown, fallback = '网络错误'): string {
  const msg = (err as { response?: { data?: { msg?: string } } })?.response
    ?.data?.msg;
  return msg || fallback;
}

export function getErrorReason(err: unknown): string {
  return (
    (err as { response?: { data?: { reason?: string } } })?.response?.data
      ?.reason ?? ''
  );
}
