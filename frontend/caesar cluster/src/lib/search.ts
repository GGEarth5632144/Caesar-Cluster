// เครื่องมือค้นหากลางของทั้งเว็บ — ใช้ร่วมกันทั้ง Topbar (ค้นข้อมูลในหน้า) และ Sidebar (ค้นเมนู)
//
// เหตุผลที่ต้องมีไฟล์นี้: เดิมแต่ละหน้าเขียน .filter(...toLowerCase().includes(...)) ของตัวเอง
// ทำให้กติกาการค้นต่างกันไปทีละหน้า และเพิ่มความสามารถทีต้องไล่แก้ทุกไฟล์
// ที่นี่กำหนดไวยากรณ์เดียวให้ทุกหน้าใช้:
//
//   apiserver              คำเปล่า — ต้องเจอในฟิลด์ใดฟิลด์หนึ่งที่หน้านั้นเปิดให้ค้น
//   "my api"               วลีที่มีช่องว่าง ต้องเจอติดกันทั้งวลี
//   status:running         เจาะจงฟิลด์
//   status:running|failed  ฟิลด์เดียวหลายค่า (เจอค่าใดค่าหนึ่งก็พอ)
//   cpu:>=500              ฟิลด์ตัวเลข เทียบด้วย > >= < <= =
//   created:>2025-01-01    ฟิลด์วันที่
//   -failed                ตัดออก (ห้ามเจอ)
//   -status:denied         ตัดออกแบบเจาะจงฟิลด์
//
// หลายเงื่อนไขที่พิมพ์ต่อกันคือ "และ" ทั้งหมด (ต้องผ่านทุกข้อ)

export type SearchFieldType = "text" | "number" | "enum" | "date";

export interface SearchFieldDef<T = any> {
  /** ชื่อที่ผู้ใช้พิมพ์หน้า ":" เช่น "status" ใน status:running */
  id: string;
  /** ชื่อที่โชว์ใน UI */
  label: string;
  type?: SearchFieldType;
  /** ดึงค่าที่จะเอาไปเทียบออกจากข้อมูล 1 แถว */
  get: (item: T) => unknown;
  /** ชื่อเรียกอื่นที่ยอมรับด้วย เช่น field "name" รับ "ชื่อ" ได้ */
  aliases?: string[];
  /** ค่าที่เป็นไปได้ของฟิลด์ประเภท enum — เอาไปทำปุ่มตัวกรองและคำแนะนำ */
  options?: { value: string; label: string }[];
  /** true = คำเปล่าๆ ไม่ค้นฟิลด์นี้ ต้องพิมพ์ "id:ค่า" เท่านั้น
   *  ใช้กับฟิลด์ที่ค่าซ้ำกันเยอะจนคำเปล่าไปแมตช์มั่ว เช่น status, role */
  exactOnly?: boolean;
  /** หน่วยของตัวเลข ใช้โชว์ในคำแนะนำเท่านั้น เช่น "millicore" */
  unit?: string;
}

export type CompareOp = "=" | ">" | ">=" | "<" | "<=" | "~";

export type SearchClause =
  | { kind: "free"; value: string; negated: boolean }
  | { kind: "field"; fieldId: string; op: CompareOp; values: string[]; negated: boolean };

export interface ParsedQuery {
  clauses: SearchClause[];
  /** คำที่ควรถูกไฮไลต์ในผลลัพธ์ (เฉพาะเงื่อนไขแบบ "ต้องเจอ") */
  highlightTerms: string[];
  /** ข้อความเตือนเมื่อพิมพ์ผิดรูป เช่น อ้างชื่อฟิลด์ที่หน้านี้ไม่มี */
  warnings: string[];
  isEmpty: boolean;
}

// ---------------------------------------------------------------------------
// ตัวตัดคำ
// ---------------------------------------------------------------------------

/** ตัดข้อความเป็น token โดยถือว่าอะไรที่อยู่ในเครื่องหมายคำพูดเป็นก้อนเดียว */
function tokenize(input: string): string[] {
  const tokens: string[] = [];
  let current = "";
  let inQuote = false;

  for (const ch of input) {
    if (ch === '"') {
      inQuote = !inQuote;
      current += ch;
      continue;
    }
    // ช่องว่างนอกเครื่องหมายคำพูดเท่านั้นที่ตัดคำ ข้างในถือเป็นตัวอักษรธรรมดา
    if (!inQuote && /\s/.test(ch)) {
      if (current) tokens.push(current);
      current = "";
      continue;
    }
    current += ch;
  }
  if (current) tokens.push(current);
  return tokens;
}

function stripQuotes(value: string): string {
  return value.replace(/"/g, "");
}

/** แยกค่าหลายค่าที่คั่นด้วย | หรือ , โดยไม่แตะตัวคั่นที่อยู่ในเครื่องหมายคำพูด */
function splitValues(raw: string): string[] {
  const parts: string[] = [];
  let current = "";
  let inQuote = false;

  for (const ch of raw) {
    if (ch === '"') {
      inQuote = !inQuote;
      continue;
    }
    if (!inQuote && (ch === "|" || ch === ",")) {
      parts.push(current);
      current = "";
      continue;
    }
    current += ch;
  }
  parts.push(current);
  return parts.map((p) => p.trim()).filter((p) => p !== "");
}

function readOperator(raw: string): { op: CompareOp; rest: string } {
  if (raw.startsWith(">=")) return { op: ">=", rest: raw.slice(2) };
  if (raw.startsWith("<=")) return { op: "<=", rest: raw.slice(2) };
  if (raw.startsWith(">")) return { op: ">", rest: raw.slice(1) };
  if (raw.startsWith("<")) return { op: "<", rest: raw.slice(1) };
  if (raw.startsWith("=")) return { op: "=", rest: raw.slice(1) };
  if (raw.startsWith("~")) return { op: "~", rest: raw.slice(1) };
  return { op: "=", rest: raw };
}

/** หา field จาก id หรือชื่อเรียกอื่น — คืน undefined ถ้าหน้านี้ไม่มีฟิลด์ชื่อนั้น */
function findField<T>(fields: SearchFieldDef<T>[], name: string): SearchFieldDef<T> | undefined {
  const lower = name.toLowerCase();
  return fields.find(
    (f) => f.id.toLowerCase() === lower || (f.aliases ?? []).some((a) => a.toLowerCase() === lower),
  );
}

// ---------------------------------------------------------------------------
// ตัวแปลงข้อความค้นหาเป็นเงื่อนไข
// ---------------------------------------------------------------------------

export function parseQuery<T>(input: string, fields: SearchFieldDef<T>[]): ParsedQuery {
  const clauses: SearchClause[] = [];
  const highlightTerms: string[] = [];
  const warnings: string[] = [];

  for (const token of tokenize(input)) {
    let body = token;
    let negated = false;

    // "-" หรือ "!" นำหน้าแปลว่าไม่เอา — แต่ต้องมีเนื้อตามหลัง ไม่งั้นถือเป็นตัวอักษรธรรมดา
    if ((body.startsWith("-") || body.startsWith("!")) && body.length > 1) {
      negated = true;
      body = body.slice(1);
    }

    // หา ":" ตัวแรกที่อยู่นอกเครื่องหมายคำพูด — ข้างในคำพูดอาจมี ":" ของเนื้อหาจริง
    let colonAt = -1;
    let inQuote = false;
    for (let i = 0; i < body.length; i++) {
      if (body[i] === '"') inQuote = !inQuote;
      else if (body[i] === ":" && !inQuote) {
        colonAt = i;
        break;
      }
    }

    if (colonAt > 0) {
      const name = stripQuotes(body.slice(0, colonAt));
      const field = findField(fields, name);
      if (field) {
        const { op, rest } = readOperator(body.slice(colonAt + 1));
        const values = splitValues(rest);
        if (values.length === 0) continue; // พิมพ์ "status:" ค้างไว้ ยังไม่ต้องกรองอะไร
        clauses.push({ kind: "field", fieldId: field.id, op, values, negated });
        if (!negated) highlightTerms.push(...values);
        continue;
      }
      // ชื่อฟิลด์ไม่มีจริง — ยังค้นแบบคำเปล่าต่อให้ (เช่น URL "http://x" ไม่ควรพัง)
      // แต่บอกผู้ใช้ไว้ว่าหน้านี้ไม่มีฟิลด์ชื่อนั้น เผื่อพิมพ์ผิด
      warnings.push(`หน้านี้ไม่มีตัวกรองชื่อ "${name}" — ค้นเป็นข้อความธรรมดาแทน`);
    }

    const value = stripQuotes(body).trim();
    if (!value) continue;
    clauses.push({ kind: "free", value, negated });
    if (!negated) highlightTerms.push(value);
  }

  return {
    clauses,
    highlightTerms,
    warnings,
    isEmpty: clauses.length === 0,
  };
}

// ---------------------------------------------------------------------------
// ตัวเทียบค่า
// ---------------------------------------------------------------------------

function toText(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (Array.isArray(value)) return value.map(toText).join(" ");
  if (typeof value === "object") {
    // env_vars และ object อื่นๆ — ค้นได้ทั้ง key และ value
    return Object.entries(value as Record<string, unknown>)
      .map(([k, v]) => `${k}=${toText(v)}`)
      .join(" ");
  }
  if (typeof value === "boolean") return value ? "true yes ใช่" : "false no ไม่";
  return String(value);
}

function toNumber(value: unknown): number | null {
  if (typeof value === "number") return Number.isFinite(value) ? value : null;
  const n = Number(String(value ?? "").replace(/,/g, "").trim());
  return Number.isFinite(n) ? n : null;
}

function toTime(value: unknown): number | null {
  if (value instanceof Date) return value.getTime();
  if (typeof value === "number") return value;
  const t = Date.parse(String(value ?? ""));
  return Number.isNaN(t) ? null : t;
}

function compare(actual: number, op: CompareOp, expected: number): boolean {
  switch (op) {
    case ">":
      return actual > expected;
    case ">=":
      return actual >= expected;
    case "<":
      return actual < expected;
    case "<=":
      return actual <= expected;
    default:
      return actual === expected;
  }
}

function matchNumberField(raw: unknown, op: CompareOp, values: string[]): boolean {
  const actual = toNumber(raw);
  if (actual === null) return false;
  return values.some((v) => {
    const expected = toNumber(v);
    // เทียบเป็นตัวเลขไม่ได้ (เช่น cpu:สอง) — ถอยไปเทียบเป็นข้อความแทนดีกว่าตัดทิ้งเงียบๆ
    if (expected === null) return toText(raw).toLowerCase().includes(v.toLowerCase());
    return compare(actual, op, expected);
  });
}

function matchDateField(raw: unknown, op: CompareOp, values: string[]): boolean {
  const actual = toTime(raw);
  if (actual === null) return false;
  return values.some((v) => {
    const expected = toTime(v);
    if (expected === null) return false;
    // "created:2025-01-05" (ไม่มีตัวเปรียบเทียบ) = ทั้งวันนั้น ไม่ใช่เที่ยงคืนเป๊ะๆ
    if (op === "=") {
      const dayEnd = expected + 24 * 60 * 60 * 1000 - 1;
      return actual >= expected && actual <= dayEnd;
    }
    return compare(actual, op, expected);
  });
}

function matchTextField(raw: unknown, values: string[]): boolean {
  const text = toText(raw).toLowerCase();
  return values.some((v) => text.includes(v.toLowerCase()));
}

function matchEnumField(raw: unknown, op: CompareOp, values: string[]): boolean {
  const text = toText(raw).toLowerCase();
  return values.some((v) => {
    const needle = v.toLowerCase();
    // enum ต้องตรงทั้งค่า ไม่งั้น "approved" จะไปโดน "denied" ไม่ได้ก็จริง
    // แต่ค่าที่เป็น prefix ของกันเอง (เช่น "run" กับ "running") จะกำกวม
    // ใครอยากค้นแบบมีบางส่วนให้ใช้ "~" เช่น status:~run
    return op === "~" ? text.includes(needle) : text === needle;
  });
}

function matchField<T>(field: SearchFieldDef<T>, item: T, op: CompareOp, values: string[]): boolean {
  const raw = field.get(item);
  switch (field.type) {
    case "number":
      return matchNumberField(raw, op, values);
    case "date":
      return matchDateField(raw, op, values);
    case "enum":
      return matchEnumField(raw, op, values);
    default:
      return matchTextField(raw, values);
  }
}

// ---------------------------------------------------------------------------
// ตัวสร้างฟังก์ชันกรอง
// ---------------------------------------------------------------------------

export interface MatcherOptions {
  /** ฟิลด์ที่ผู้ใช้ปิดไว้ — คำเปล่าจะไม่ค้นฟิลด์เหล่านี้ (แต่ "id:ค่า" ยังใช้ได้อยู่) */
  disabledFieldIds?: string[];
}

/**
 * สร้างฟังก์ชันเช็คว่าข้อมูล 1 แถวผ่านเงื่อนไขที่พิมพ์มาหรือไม่
 * คิวรีว่าง = ผ่านทุกแถว (ไม่กรองอะไรเลย)
 */
export function buildMatcher<T>(
  parsed: ParsedQuery,
  fields: SearchFieldDef<T>[],
  options: MatcherOptions = {},
): (item: T) => boolean {
  if (parsed.isEmpty) return () => true;

  const disabled = new Set(options.disabledFieldIds ?? []);
  const byId = new Map(fields.map((f) => [f.id, f]));
  // ฟิลด์ที่คำเปล่าจะไปค้น: ต้องไม่ถูกปิด และไม่ใช่ฟิลด์ที่ตั้ง exactOnly ไว้
  const freeFields = fields.filter((f) => !f.exactOnly && !disabled.has(f.id));

  return (item: T) => {
    for (const clause of parsed.clauses) {
      let hit: boolean;

      if (clause.kind === "free") {
        const needle = clause.value.toLowerCase();
        hit = freeFields.some((f) => toText(f.get(item)).toLowerCase().includes(needle));
      } else {
        const field = byId.get(clause.fieldId);
        hit = field ? matchField(field, item, clause.op, clause.values) : false;
      }

      // เงื่อนไขทุกข้อต้องผ่านหมด — เจอข้อที่ตกก็ตัดทิ้งได้เลยไม่ต้องเช็คต่อ
      if (clause.negated ? hit : !hit) return false;
    }
    return true;
  };
}

/** ทางลัดสำหรับกรอง array ในครั้งเดียว */
export function searchItems<T>(
  items: T[],
  query: string,
  fields: SearchFieldDef<T>[],
  options: MatcherOptions = {},
): { results: T[]; parsed: ParsedQuery } {
  const parsed = parseQuery(query, fields);
  if (parsed.isEmpty) return { results: items, parsed };
  return { results: items.filter(buildMatcher(parsed, fields, options)), parsed };
}

// ---------------------------------------------------------------------------
// ไฮไลต์คำที่ค้นเจอ
// ---------------------------------------------------------------------------

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** ตัดข้อความเป็นชิ้นๆ พร้อมบอกว่าชิ้นไหนควรถูกไฮไลต์ */
export function highlightParts(text: string, terms: string[]): { text: string; hit: boolean }[] {
  const usable = terms.map((t) => t.trim()).filter((t) => t.length > 0);
  if (usable.length === 0 || !text) return [{ text, hit: false }];

  // capture group ทำให้ String.split เก็บตัวคั่นไว้ด้วย ชิ้นที่เป็นตัวคั่นคือชิ้นที่ตรงคำค้น
  const splitter = new RegExp(`(${usable.map(escapeRegExp).join("|")})`, "gi");
  const lowered = new Set(usable.map((t) => t.toLowerCase()));

  return text
    .split(splitter)
    .filter((piece) => piece !== "")
    .map((piece) => ({ text: piece, hit: lowered.has(piece.toLowerCase()) }));
}

// ---------------------------------------------------------------------------
// คะแนนความใกล้เคียง — ใช้กับการค้นเมนู/ชื่อหน้า ที่ต้องเรียงลำดับผลลัพธ์
// ---------------------------------------------------------------------------

/**
 * ให้คะแนนว่า query ใกล้เคียงกับ text แค่ไหน (ยิ่งมากยิ่งตรง, 0 = ไม่ตรงเลย)
 * เรียงจากตรงที่สุดลงมา: ตรงทั้งคำ > ขึ้นต้นด้วย > มีคำนี้อยู่ > ตัวอักษรเรียงตามลำดับ
 */
export function fuzzyScore(text: string, query: string): number {
  const haystack = text.toLowerCase();
  const needle = query.trim().toLowerCase();
  if (!needle) return 1;
  if (haystack === needle) return 1000;
  if (haystack.startsWith(needle)) return 800 - haystack.length;

  const at = haystack.indexOf(needle);
  if (at >= 0) {
    // ขึ้นต้นคำใดคำหนึ่ง (หลังช่องว่าง/ขีด) ถือว่าตรงกว่าโผล่กลางคำ
    const atWordStart = at === 0 || /[\s\-_/]/.test(haystack[at - 1]);
    return (atWordStart ? 600 : 400) - at;
  }

  // subsequence: พิมพ์ "usmg" แล้วยังเจอ "User Management"
  let cursor = 0;
  let score = 0;
  let streak = 0;
  for (const ch of needle) {
    const found = haystack.indexOf(ch, cursor);
    if (found === -1) return 0;
    streak = found === cursor ? streak + 1 : 0;
    score += 10 + streak * 5;
    cursor = found + 1;
  }
  return score;
}
