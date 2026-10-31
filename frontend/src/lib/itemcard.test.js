import { describe, expect, it } from "vitest";
import {
  cleanParams,
  filterMoves,
  imageProblem,
  moveImage,
  moveLabel,
  validEAN,
} from "./itemcard.js";

const moves = [
  {
    number: "PR-1",
    kind: "receipt",
    partner: "Elektro Žilina",
    warehouse: "HL",
    quantity: "20",
    date: "2026-10-01",
  },
  {
    number: "PS-1",
    kind: "transfer",
    warehouse: "HL",
    quantity: "-8",
    date: "2026-10-05",
  },
  {
    number: "PS-1",
    kind: "transfer",
    warehouse: "AUTO",
    quantity: "8",
    date: "2026-10-05",
  },
  {
    number: "VY-1",
    kind: "issue",
    partner: "Novák",
    warehouse: "AUTO",
    quantity: "-2",
    date: "2026-10-07",
    return: false,
  },
];

describe("filterMoves", () => {
  it.each([
    [{}, 4],
    [{ search: "zilina" }, 1],
    [{ search: "presun prijem" }, 1],
    [{ warehouse: "AUTO" }, 2],
    [{ warehouse: "AUTO", search: "novak" }, 1],
    [{ search: "nič" }, 0],
  ])("%o", (f, n) => expect(filterMoves(moves, f)).toHaveLength(n));
});

it("moveLabel", () => {
  expect(moves.map(moveLabel)).toEqual([
    "Príjemka",
    "Presun – výdaj",
    "Presun – príjem",
    "Výdajka",
  ]);
});

it("cleanParams", () => {
  expect(
    cleanParams([
      { name: " Napätie ", value: "24", unit: "V " },
      { name: "", value: " ", unit: "" },
    ]),
  ).toEqual([{ name: "Napätie", value: "24", unit: "V" }]);
});

it.each([
  [[1, 2, 3], 2, 0, [3, 1, 2]],
  [[1, 2, 3], 0, 1, [2, 1, 3]],
  [[1, 2, 3], 0, -1, [1, 2, 3]],
  [[1, 2, 3], 2, 3, [1, 2, 3]],
])("moveImage %o %i→%i", (ids, from, to, want) =>
  expect(moveImage(ids, from, to)).toEqual(want),
);

it.each([
  [{ name: "a.png", type: "image/png", size: 1000 }, 0, null],
  [
    { name: "a.pdf", type: "application/pdf", size: 1000 },
    0,
    "a.pdf: iba JPEG, PNG alebo WebP.",
  ],
  [{ name: "a.jpg", type: "image/jpeg", size: 3e6 }, 0, "a.jpg: najviac 2 MB."],
  [
    { name: "a.jpg", type: "image/jpeg", size: 10 },
    10,
    "Produkt môže mať najviac 10 obrázkov.",
  ],
])("imageProblem %o", (f, n, want) => expect(imageProblem(f, n)).toBe(want));

it.each([
  ["5901234123457", true],
  ["5901234123458", false],
  ["96385074", true],
  ["036000291452", true],
  ["12345", false],
  ["59012341234a7", false],
])("validEAN %s", (s, ok) => expect(validEAN(s)).toBe(ok));
