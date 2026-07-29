import c01 from "./c01.mjs";
import c02 from "./c02.mjs";
import c03 from "./c03.mjs";
import c04 from "./c04.mjs";
import c05 from "./c05.mjs";
import c06 from "./c06.mjs";
import i01 from "./i01.mjs";
import i02 from "./i02.mjs";
import i03 from "./i03.mjs";
import i04 from "./i04.mjs";
import i05 from "./i05.mjs";
import i06 from "./i06.mjs";
import i07 from "./i07.mjs";
import i08 from "./i08.mjs";
import i09 from "./i09.mjs";
import i10 from "./i10.mjs";
import l01 from "./l01.mjs";
import l02 from "./l02.mjs";
import l03 from "./l03.mjs";
import l04 from "./l04.mjs";
import l05 from "./l05.mjs";
import l06 from "./l06.mjs";
import p01 from "./p01.mjs";
import p02 from "./p02.mjs";
import p03 from "./p03.mjs";
import p04 from "./p04.mjs";
import r01 from "./r01.mjs";
import r02 from "./r02.mjs";
import r03 from "./r03.mjs";
import r04 from "./r04.mjs";
import r05 from "./r05.mjs";
import r06 from "./r06.mjs";
import r07 from "./r07.mjs";

export const recordScenarioDefinitions = Object.freeze({
  c01,
  c02,
  c03,
  c04,
  c05,
  c06,
  i01,
  i02,
  i03,
  i04,
  i05,
  i06,
  i07,
  i08,
  i09,
  i10,
  r01,
  r02,
  r03,
  r04,
  r05,
  r06,
  r07,
  l01,
  l02,
  l03,
  l04,
  l05,
  l06,
  p01,
  p02,
  p03,
  p04
});

for (const [id, scenario] of Object.entries(recordScenarioDefinitions)) {
  if (scenario.id !== id) {
    throw new Error(
      `record scenario key ${id} does not match id ${scenario.id}`
    );
  }
}

export const recordScenarioIds = Object.freeze(
  Object.keys(recordScenarioDefinitions)
);
