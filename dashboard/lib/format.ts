export const usd = (n: number): string => {
  if (!n) return "$0.00";
  if (Math.abs(n) < 0.01) return "$" + n.toFixed(6);
  return "$" + n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
};

export const num = (n: number): string => (n ?? 0).toLocaleString("en-US");

export const pct = (n: number): string => (n * 100).toFixed(1) + "%";

export const ms = (n: number): string => num(n) + " ms";

// Compact thousands (1.2k) for dense labels.
export const compact = (n: number): string => {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return (n / 1000).toFixed(n % 1000 === 0 ? 0 : 1) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
};
