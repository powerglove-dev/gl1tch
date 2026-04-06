import { useState } from "react";
import { ArrowUpDown } from "lucide-react";

interface Props {
  headers: string[];
  rows: string[][];
}

export function TableBlock({ headers, rows }: Props) {
  const [sortCol, setSortCol] = useState<number | null>(null);
  const [sortAsc, setSortAsc] = useState(true);

  function handleSort(col: number) {
    if (sortCol === col) {
      setSortAsc(!sortAsc);
    } else {
      setSortCol(col);
      setSortAsc(true);
    }
  }

  const sorted =
    sortCol !== null
      ? [...rows].sort((a, b) => {
          const va = a[sortCol] ?? "";
          const vb = b[sortCol] ?? "";
          const cmp = va.localeCompare(vb, undefined, { numeric: true });
          return sortAsc ? cmp : -cmp;
        })
      : rows;

  return (
    <div className="my-3 rounded-lg border border-surface overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full text-[13px]">
          <thead>
            <tr className="bg-bg-darker">
              {headers.map((h, i) => (
                <th
                  key={i}
                  onClick={() => handleSort(i)}
                  className="text-left px-4 py-2.5 text-cyan text-[11px] font-semibold uppercase tracking-wider cursor-pointer hover:bg-surface transition-colors select-none"
                >
                  <span className="flex items-center gap-1.5">
                    {h}
                    <ArrowUpDown
                      size={11}
                      className={
                        sortCol === i ? "text-cyan" : "text-comment opacity-40"
                      }
                    />
                  </span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {sorted.map((row, ri) => (
              <tr
                key={ri}
                className="border-t border-surface hover:bg-surface/50 transition-colors"
              >
                {row.map((cell, ci) => (
                  <td key={ci} className="px-4 py-2 text-fg">
                    {cell}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="px-4 py-1.5 bg-bg-darker border-t border-surface text-[11px] text-comment">
        {rows.length} row{rows.length !== 1 ? "s" : ""}
      </div>
    </div>
  );
}
