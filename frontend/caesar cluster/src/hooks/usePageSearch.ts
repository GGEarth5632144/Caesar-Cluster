import { useDeferredValue, useEffect, useMemo, useRef } from "react";

import { buildMatcher, parseQuery, type ParsedQuery } from "@/lib/search";
import { useSearchStore, type SearchScope } from "@/store/searchStore";

export interface PageSearchResult<T> {
  /** ข้อมูลที่ผ่านเงื่อนไขแล้ว — เอาไป map เป็นแถวได้เลย */
  results: T[];
  /** ข้อความที่ผู้ใช้พิมพ์อยู่ตอนนี้ (ยังไม่หน่วง) เอาไว้ใช้กับข้อความ empty state */
  query: string;
  /** true = กำลังกรองอยู่ ใช้แยก "ยังไม่มีข้อมูล" ออกจาก "ค้นแล้วไม่เจอ" */
  isFiltering: boolean;
  /** คำที่ควรไฮไลต์ในผลลัพธ์ ส่งต่อให้ <Highlight/> ได้ตรงๆ */
  highlightTerms: string[];
  parsed: ParsedQuery;
}

/**
 * ผูกหน้าเข้ากับช่องค้นหาบน Topbar
 *
 * เรียกครั้งเดียวในหน้า: บอกว่าหน้านี้ค้นอะไรได้ (scope) และมีข้อมูลอะไรอยู่ (items)
 * แล้วรับข้อมูลที่กรองแล้วกลับไปใช้ ไม่ต้องมีช่องค้นหาของตัวเองอีก
 *
 * scope ต้องเป็นค่าคงที่ (ประกาศไว้นอกคอมโพเนนต์ หรือห่อ useMemo)
 * เพราะ effect ที่ register ใช้ scope เป็น dependency — ถ้าสร้างใหม่ทุก render
 * มันจะ register/unregister วนไม่จบ
 */
export function usePageSearch<T>(scope: SearchScope, items: T[]): PageSearchResult<T> {
  const query = useSearchStore((state) => state.query);
  const registerScope = useSearchStore((state) => state.registerScope);
  const unregisterScope = useSearchStore((state) => state.unregisterScope);
  const reportCount = useSearchStore((state) => state.reportCount);
  const disabledFieldIds = useSearchStore((state) => state.disabledFields[scope.id]);

  useEffect(() => {
    registerScope(scope);
    return () => unregisterScope(scope.id);
  }, [scope, registerScope, unregisterScope]);

  // useDeferredValue ทำให้การพิมพ์ยังลื่นแม้ตารางจะยาว: React render ตัวอักษรที่พิมพ์ก่อน
  // แล้วค่อยตามมากรองรายการทีหลัง (ไม่ต้องตั้ง setTimeout debounce เอง)
  const deferredQuery = useDeferredValue(query);

  const { results, parsed } = useMemo(() => {
    const parsedQuery = parseQuery(deferredQuery, scope.fields);
    if (parsedQuery.isEmpty) return { results: items, parsed: parsedQuery };
    const matcher = buildMatcher(parsedQuery, scope.fields, { disabledFieldIds });
    return { results: items.filter(matcher), parsed: parsedQuery };
  }, [items, deferredQuery, scope.fields, disabledFieldIds]);

  // รายงานจำนวนให้ Topbar โชว์ — เก็บใน ref ด้วยเพื่อไม่ยิง set ซ้ำเมื่อตัวเลขเท่าเดิม
  const lastReported = useRef<[number, number] | null>(null);
  useEffect(() => {
    const pair: [number, number] = [results.length, items.length];
    const previous = lastReported.current;
    if (previous && previous[0] === pair[0] && previous[1] === pair[1]) return;
    lastReported.current = pair;
    reportCount(pair[0], pair[1]);
  }, [results.length, items.length, reportCount]);

  return {
    results,
    query,
    isFiltering: !parsed.isEmpty,
    highlightTerms: parsed.highlightTerms,
    parsed,
  };
}
