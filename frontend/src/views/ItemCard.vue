<script setup>
// Karta produktu: údaje z cenníka, obrázky, parametre, stav po skladoch a
// všetky pohyby. Stavy, ocenenie a ceny s DPH počíta backend.
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useToast } from "primevue/usetoast";
import { useConfirm } from "primevue/useconfirm";
import Button from "primevue/button";
import Checkbox from "primevue/checkbox";
import InputText from "primevue/inputtext";
import InputNumber from "primevue/inputnumber";
import Select from "primevue/select";
import SelectButton from "primevue/selectbutton";
import Textarea from "primevue/textarea";
import ToggleSwitch from "primevue/toggleswitch";
import IconField from "primevue/iconfield";
import InputIcon from "primevue/inputicon";
import FormField from "../components/FormField.vue";
import ExportButton from "../components/ExportButton.vue";
import { api, ApiError } from "../lib/api.js";
import { session, can } from "../lib/session.js";
import { codes, ratesOn } from "../lib/codes.js";
import { formatAmount, formatDate, formatQuantity } from "../lib/format.js";
import { itemKindLabels } from "../lib/stock.js";
import { itemMoveColumns } from "../lib/columns.js";
import {
  blankParam,
  cleanParams,
  filterMoves,
  imageProblem,
  imageTypes,
  maxImages,
  moveImage,
  moveLabel,
  validEAN,
} from "../lib/itemcard.js";

const props = defineProps({ id: { type: String, required: true } });
const route = useRoute();
const router = useRouter();
const toast = useToast();
const confirm = useConfirm();

const canWrite = computed(() => can("invoices.write"));
const canStock = computed(() => can("purchases.read"));
const canStockWrite = computed(() => can("purchases.write"));

const card = ref(null);
const form = ref(null);
const stockForm = ref(null);
const errors = ref({});
const busy = ref(false);
const loading = ref(true);
const notFound = ref(false);

const units = computed(() =>
  session.company?.units?.length ? session.company.units : codes.defaultUnits,
);
const nonVat = computed(() => !session.company?.vatPayer);
const rates = computed(() => ratesOn(new Date().toISOString().slice(0, 10)));
const priceModes = [
  { label: "bez DPH", value: "net" },
  { label: "s DPH", value: "gross" },
];
const kindOptions = Object.entries(itemKindLabels).map(([value, label]) => ({
  value,
  label,
}));

// sumy z API sú reťazce; do InputNumber idú čísla, späť zase reťazce
const num = (s) =>
  s === undefined || s === null || s === "" ? null : Number(s);
const str = (n) => (n === null || n === undefined ? "0" : String(n));

function fill(c) {
  card.value = c;
  const it = c.item;
  form.value = {
    ...it,
    price: num(it.price),
    purchasePrice: Number(it.purchasePrice) ? num(it.purchasePrice) : null,
    weight: Number(it.weight) ? num(it.weight) : null,
    params: (it.params ?? []).map((p) => ({ ...p })),
  };
  if (!form.value.params.length) form.value.params.push(blankParam());
  stockForm.value = {
    tracked: c.tracked,
    kind: c.kind || "goods",
    min: Number(c.min) ? num(c.min) : null,
  };
}

async function load() {
  loading.value = true;
  notFound.value = false;
  try {
    fill(await api.itemCard(session.companyId, props.id));
  } catch (e) {
    if (e.status === 404) notFound.value = true;
    else
      toast.add({
        severity: "error",
        summary: "Karta sa nenačítala",
        detail: e.message,
        life: 6000,
      });
  } finally {
    loading.value = false;
  }
}
onMounted(load);
watch(() => props.id, load);

const stockDirty = computed(() => {
  if (!card.value || !stockForm.value) return false;
  const s = stockForm.value;
  return (
    s.tracked !== card.value.tracked ||
    (s.tracked &&
      (s.kind !== (card.value.kind || "goods") ||
        str(s.min) !== str(num(card.value.min))))
  );
});
const eanWarn = computed(() => {
  const e = form.value?.ean?.trim();
  return e && !validEAN(e) ? "Kontrolná číslica nesedí" : "";
});

async function save() {
  busy.value = true;
  errors.value = {};
  try {
    const f = form.value;
    const saved = await api.saveItem(session.companyId, {
      ...f,
      price: str(f.price),
      purchasePrice: str(f.purchasePrice),
      weight: str(f.weight),
      params: cleanParams(f.params),
    });
    let next = { ...card.value, item: saved };
    if (stockDirty.value && canStockWrite.value) {
      const s = stockForm.value;
      next = await api.setItemStock(session.companyId, f.id, {
        tracked: s.tracked,
        kind: s.kind,
        min: str(s.min),
      });
    }
    fill(next);
    toast.add({ severity: "success", summary: "Karta uložená", life: 2500 });
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap();
      toast.add({
        severity: "error",
        summary: "Skontrolujte vyznačené polia",
        detail: e.fields[0].message,
        life: 5000,
      });
    } else {
      toast.add({
        severity: "error",
        summary: "Uloženie zlyhalo",
        detail: e.message,
        life: 6000,
      });
    }
  } finally {
    busy.value = false;
  }
}

function paramError(i, key) {
  return (
    errors.value[`params[${i}].${key}`] ||
    (key === "name" ? errors.value[`params[${i}]`] : "")
  );
}

// ---- obrázky ----
const imgInput = ref(null);
const active = ref(0); // index obrázka vo veľkom náhľade
const uploading = ref(false);
const dragging = ref(false);
const images = computed(() => card.value?.images ?? []);
const imgURL = (im) => api.itemImageURL(session.companyId, props.id, im.id);
watch(images, (list) => {
  if (active.value >= list.length) active.value = Math.max(0, list.length - 1);
});

async function upload(files) {
  uploading.value = true;
  try {
    for (const file of files) {
      const problem = imageProblem(file, images.value.length);
      if (problem) {
        toast.add({
          severity: "warn",
          summary: "Obrázok sa nepridal",
          detail: problem,
          life: 5000,
        });
        continue;
      }
      try {
        const im = await api.addItemImage(session.companyId, props.id, file);
        card.value = { ...card.value, images: [...images.value, im] };
        active.value = images.value.length - 1;
      } catch (e) {
        const msg =
          e instanceof ApiError && e.fields.length
            ? e.fields[0].message
            : e.message;
        toast.add({
          severity: "error",
          summary: `${file.name} sa nepridal`,
          detail: msg,
          life: 6000,
        });
      }
    }
  } finally {
    uploading.value = false;
  }
}
function pickImages(e) {
  const files = [...(e.target.files ?? [])];
  e.target.value = "";
  if (files.length) upload(files);
}
function onDrop(e) {
  dragging.value = false;
  if (!canWrite.value) return;
  const files = [...(e.dataTransfer?.files ?? [])].filter((f) =>
    f.type.startsWith("image/"),
  );
  if (files.length) upload(files);
}

async function reorder(from, to) {
  const before = images.value.map((im) => im.id);
  const ids = moveImage(before, from, to);
  if (ids === before) return;
  try {
    const list = await api.orderItemImages(session.companyId, props.id, ids);
    card.value = { ...card.value, images: list };
    active.value = to;
  } catch (e) {
    toast.add({
      severity: "error",
      summary: "Poradie sa nezmenilo",
      detail: e.message,
      life: 6000,
    });
  }
}

function removeImage(im) {
  confirm.require({
    header: "Zmazať obrázok",
    message: `Obrázok ${im.name} sa z karty odstráni.`,
    acceptProps: { label: "Zmazať", severity: "danger" },
    rejectProps: { label: "Ponechať", severity: "secondary", outlined: true },
    accept: async () => {
      try {
        await api.deleteItemImage(session.companyId, props.id, im.id);
        card.value = {
          ...card.value,
          images: images.value.filter((x) => x.id !== im.id),
        };
      } catch (e) {
        toast.add({
          severity: "error",
          summary: "Zmazanie zlyhalo",
          detail: e.message,
          life: 6000,
        });
      }
    },
  });
}

// ---- pohyby ----
const search = ref("");
const ALL = "*"; // Select nevie zobraziť prázdnu hodnotu ako voľbu
const warehouse = ref(
  typeof route.query.sklad === "string" && route.query.sklad
    ? route.query.sklad
    : ALL,
);
const warehouseOptions = computed(() => [
  { code: ALL, name: "Všetky sklady" },
  ...(card.value?.warehouses ?? []),
]);
const multi = computed(() => (card.value?.warehouses?.length ?? 0) > 1);
const moves = computed(() =>
  filterMoves(card.value?.moves, {
    search: search.value,
    warehouse: warehouse.value === ALL ? "" : warehouse.value,
  }),
);
const qty = (v) => formatQuantity(v);

function remove() {
  confirm.require({
    header: "Zmazať položku",
    message: `${form.value.name} sa odstráni z cenníka aj s obrázkami. Faktúry, na ktorých je, sa nezmenia.`,
    acceptProps: { label: "Zmazať", severity: "danger" },
    rejectProps: { label: "Ponechať", severity: "secondary", outlined: true },
    accept: async () => {
      try {
        await api.deleteItem(session.companyId, form.value.id);
        toast.add({ severity: "info", summary: "Položka zmazaná", life: 2500 });
        router.push("/cennik");
      } catch (e) {
        toast.add({
          severity: "error",
          summary: "Zmazanie zlyhalo",
          detail: e.message,
          life: 6000,
        });
      }
    },
  });
}

function onKey(e) {
  if (
    (e.ctrlKey || e.metaKey) &&
    e.key === "s" &&
    form.value &&
    canWrite.value
  ) {
    e.preventDefault();
    if (!busy.value) save();
  }
}
onMounted(() => window.addEventListener("keydown", onKey));
onBeforeUnmount(() => window.removeEventListener("keydown", onKey));
</script>

<template>
  <div class="itemcard">
    <p v-if="notFound" class="missing">
      Položka neexistuje. <RouterLink to="/cennik">Späť do cenníka</RouterLink>
    </p>
    <p v-else-if="loading && !card" class="missing">Načítavam kartu…</p>

    <template v-else-if="card && form">
      <header class="head">
        <RouterLink to="/cennik" class="back" aria-label="Späť do cenníka"
          ><i class="pi pi-arrow-left"
        /></RouterLink>
        <div class="title">
          <span class="crumb"
            >Cenník<template v-if="form.category">
              / {{ form.category }}</template
            ></span
          >
          <h1>
            <span v-if="card.item.code" class="code">{{ card.item.code }}</span
            >{{ card.item.name }}
          </h1>
        </div>
        <span
          v-if="card.tracked"
          class="tag"
          :data-tone="card.below ? 'overdue' : 'paid'"
        >
          <span class="tag-text">{{
            card.below ? "pod minimom" : itemKindLabels[card.kind]
          }}</span>
        </span>
        <span v-if="card.item.archived" class="tag" data-tone="cancelled"
          ><span class="tag-text">archivovaná</span></span
        >
        <div class="spacer" />
        <Button
          v-if="canWrite"
          label="Zmazať"
          severity="danger"
          text
          size="small"
          class="hide-m"
          @click="remove"
        />
        <Button
          v-if="canWrite"
          label="Uložiť"
          icon="pi pi-check"
          size="small"
          :loading="busy"
          @click="save"
        />
      </header>

      <!-- súhrn ako riadok v knihe: ceny a zásoba -->
      <dl class="ledger">
        <div>
          <dt>Predaj bez DPH</dt>
          <dd>{{ formatAmount(card.item.priceNet) }} €</dd>
        </div>
        <div v-if="!nonVat">
          <dt>Predaj s DPH</dt>
          <dd>{{ formatAmount(card.item.priceGross) }} €</dd>
        </div>
        <div>
          <dt>Nákup bez DPH</dt>
          <dd>
            {{
              Number(card.item.purchasePrice)
                ? formatAmount(card.item.purchasePrice) + " €"
                : "—"
            }}
          </dd>
        </div>
        <template v-if="canStock && card.tracked">
          <div>
            <dt>Na sklade</dt>
            <dd :class="{ low: card.below }">
              {{ qty(card.quantity) }} {{ card.item.unit }}
            </dd>
          </div>
          <div>
            <dt>Hodnota zásoby</dt>
            <dd class="total">{{ formatAmount(card.value) }} €</dd>
          </div>
        </template>
      </dl>

      <div class="body">
        <!-- ľavý stĺpec: obrázky a sklad -->
        <div class="col-side">
          <section
            class="sheet gallery"
            aria-label="Obrázky produktu"
            :class="{ drag: dragging }"
            @dragover.prevent="dragging = canWrite"
            @dragleave="dragging = false"
            @drop.prevent="onDrop"
          >
            <div class="stage">
              <img
                v-if="images.length"
                :src="imgURL(images[active])"
                :alt="images[active].name"
              />
              <div v-else class="empty-img">
                <i class="pi pi-image" aria-hidden="true" />
                <span>{{
                  canWrite ? "Pretiahnite sem fotky produktu" : "Bez obrázka"
                }}</span>
              </div>
              <span v-if="images.length && active === 0" class="main-mark"
                >hlavný</span
              >
            </div>
            <div v-if="images.length" class="thumbs" role="list">
              <button
                v-for="(im, i) in images"
                :key="im.id"
                type="button"
                role="listitem"
                class="thumb"
                :class="{ on: i === active }"
                :aria-label="`Obrázok ${i + 1}: ${im.name}`"
                @click="active = i"
              >
                <img :src="imgURL(im)" alt="" loading="lazy" />
              </button>
            </div>
            <div v-if="canWrite" class="img-actions">
              <input
                ref="imgInput"
                type="file"
                :accept="imageTypes.join(',')"
                multiple
                class="hidden"
                @change="pickImages"
              />
              <Button
                icon="pi pi-plus"
                label="Pridať"
                size="small"
                severity="secondary"
                outlined
                :loading="uploading"
                :disabled="images.length >= maxImages"
                @click="imgInput.click()"
              />
              <template v-if="images.length">
                <Button
                  v-if="active > 0"
                  icon="pi pi-star"
                  text
                  size="small"
                  severity="secondary"
                  v-tooltip.bottom="'Nastaviť ako hlavný'"
                  aria-label="Nastaviť ako hlavný"
                  @click="reorder(active, 0)"
                />
                <Button
                  icon="pi pi-angle-left"
                  text
                  size="small"
                  severity="secondary"
                  :disabled="active === 0"
                  aria-label="Posunúť doľava"
                  @click="reorder(active, active - 1)"
                />
                <Button
                  icon="pi pi-angle-right"
                  text
                  size="small"
                  severity="secondary"
                  :disabled="active === images.length - 1"
                  aria-label="Posunúť doprava"
                  @click="reorder(active, active + 1)"
                />
                <span class="spacer" />
                <Button
                  icon="pi pi-trash"
                  text
                  size="small"
                  severity="danger"
                  aria-label="Zmazať obrázok"
                  @click="removeImage(images[active])"
                />
              </template>
            </div>
            <p class="note">
              {{ images.length }} / {{ maxImages }} · JPEG, PNG, WebP do 2 MB.
              Prvý obrázok je hlavný (zoznam, e-shop).
            </p>
          </section>

          <section v-if="canStock" class="sheet pad stock">
            <h2>Sklad</h2>
            <label class="switch">
              <ToggleSwitch
                v-model="stockForm.tracked"
                :disabled="!canStockWrite"
              />
              <span
                >Vediem na sklade<small
                  >Faktúra vydá zo skladu, nákup prijme.</small
                ></span
              >
            </label>
            <div v-if="stockForm.tracked" class="form-grid two">
              <FormField label="Druh zásoby" :error="errors.kind">
                <Select
                  v-model="stockForm.kind"
                  :options="kindOptions"
                  option-label="label"
                  option-value="value"
                  size="small"
                  :disabled="!canStockWrite"
                />
              </FormField>
              <FormField :label="`Minimum (${form.unit})`" :error="errors.min">
                <InputNumber
                  v-model="stockForm.min"
                  :min="0"
                  :max-fraction-digits="3"
                  locale="sk-SK"
                  size="small"
                  :disabled="!canStockWrite"
                />
              </FormField>
            </div>
            <p v-if="errors.items" class="err">{{ errors.items }}</p>
            <p v-if="stockDirty" class="hint">
              Zmena skladu sa uloží s kartou.
            </p>

            <table v-if="card.tracked && card.warehouses.length" class="wh">
              <thead>
                <tr>
                  <th>Sklad</th>
                  <th class="num">Množstvo</th>
                  <th class="num">Ø cena</th>
                  <th class="num">Hodnota</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="w in card.warehouses"
                  :key="w.code"
                  :class="{ on: warehouse === w.code }"
                  tabindex="0"
                  @click="warehouse = warehouse === w.code ? ALL : w.code"
                  @keydown.enter="
                    warehouse = warehouse === w.code ? ALL : w.code
                  "
                >
                  <td>
                    <b>{{ w.code }}</b> {{ w.name }}
                  </td>
                  <td class="num" :class="{ neg: Number(w.quantity) < 0 }">
                    {{ qty(w.quantity) }}
                  </td>
                  <td class="num">{{ formatAmount(w.avgCost) }}</td>
                  <td class="num">{{ formatAmount(w.value) }}</td>
                </tr>
              </tbody>
              <tfoot v-if="multi">
                <tr>
                  <td>Spolu</td>
                  <td class="num">{{ qty(card.quantity) }}</td>
                  <td />
                  <td class="num total">{{ formatAmount(card.value) }}</td>
                </tr>
              </tfoot>
            </table>
            <p v-if="card.tracked" class="hint">
              Ocenenie:
              {{ card.method === "fifo" ? "FIFO" : "vážený priemer" }}. Príjem,
              výdaj a presun zapíšete v
              <RouterLink to="/sklad">Sklade</RouterLink>.
            </p>
          </section>
        </div>

        <!-- pravý stĺpec: údaje karty -->
        <div class="col-main">
          <fieldset class="sheet pad form-section">
            <legend>Základné údaje</legend>
            <div class="form-grid">
              <FormField
                label="Názov (text na faktúre)"
                :span="4"
                :error="errors.name"
              >
                <InputText
                  v-model="form.name"
                  size="small"
                  :invalid="!!errors.name"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField label="Kód" :error="errors.code">
                <InputText
                  v-model="form.code"
                  size="small"
                  :invalid="!!errors.code"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField
                label="EAN"
                :error="errors.ean || eanWarn"
                hint="čiarový kód"
              >
                <InputText
                  v-model="form.ean"
                  size="small"
                  inputmode="numeric"
                  :invalid="!!(errors.ean || eanWarn)"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField
                label="Kategória"
                :span="2"
                :error="errors.category"
                hint="napr. Káble / Silové"
              >
                <InputText
                  v-model="form.category"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField
                label="Značka / výrobca"
                :span="2"
                :error="errors.brand"
              >
                <InputText
                  v-model="form.brand"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField label="Merná jednotka" :error="errors.unit">
                <Select
                  v-model="form.unit"
                  :options="units"
                  editable
                  size="small"
                  :invalid="!!errors.unit"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField label="Hmotnosť (kg)" :error="errors.weight">
                <InputNumber
                  v-model="form.weight"
                  :min="0"
                  :max-fraction-digits="3"
                  locale="sk-SK"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
            </div>
          </fieldset>

          <fieldset class="sheet pad form-section">
            <legend>Ceny a nákup</legend>
            <div class="form-grid">
              <FormField label="Predajná cena za MJ" :error="errors.price">
                <InputNumber
                  v-model="form.price"
                  :min-fraction-digits="2"
                  :max-fraction-digits="4"
                  locale="sk-SK"
                  suffix=" €"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField
                v-if="!nonVat"
                label="Cena je"
                :error="errors.priceMode"
              >
                <SelectButton
                  v-model="form.priceMode"
                  :options="priceModes"
                  option-label="label"
                  option-value="value"
                  :allow-empty="false"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField
                v-if="!nonVat"
                label="Sadzba DPH"
                :error="errors.vatRate"
              >
                <Select
                  v-model="form.vatRate"
                  :options="rates"
                  size="small"
                  :disabled="!canWrite"
                >
                  <template #value="{ value }">{{ value }} %</template>
                  <template #option="{ option }">{{ option }} %</template>
                </Select>
              </FormField>
              <FormField
                label="Nákupná cena bez DPH"
                :error="errors.purchasePrice"
                hint="posledná"
              >
                <InputNumber
                  v-model="form.purchasePrice"
                  :min="0"
                  :min-fraction-digits="2"
                  :max-fraction-digits="4"
                  locale="sk-SK"
                  suffix=" €"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField label="Dodávateľ" :span="2" :error="errors.supplier">
                <InputText
                  v-model="form.supplier"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField
                label="Kód u dodávateľa"
                :span="2"
                :error="errors.supplierCode"
              >
                <InputText
                  v-model="form.supplierCode"
                  size="small"
                  :disabled="!canWrite"
                />
              </FormField>
            </div>
          </fieldset>

          <fieldset class="sheet pad form-section">
            <legend>Parametre</legend>
            <div class="params" role="table" aria-label="Parametre produktu">
              <div class="p-row p-head" role="row">
                <span role="columnheader">Parameter</span
                ><span role="columnheader">Hodnota</span
                ><span role="columnheader">Jednotka</span><span />
              </div>
              <div
                v-for="(p, i) in form.params"
                :key="i"
                class="p-row"
                role="row"
              >
                <InputText
                  v-model="p.name"
                  size="small"
                  :placeholder="i === 0 ? 'Napätie' : ''"
                  :invalid="!!paramError(i, 'name')"
                  :disabled="!canWrite"
                  :aria-label="`Parameter ${i + 1}`"
                />
                <InputText
                  v-model="p.value"
                  size="small"
                  :placeholder="i === 0 ? '230' : ''"
                  :disabled="!canWrite"
                  :aria-label="`Hodnota ${i + 1}`"
                />
                <InputText
                  v-model="p.unit"
                  size="small"
                  :placeholder="i === 0 ? 'V' : ''"
                  :disabled="!canWrite"
                  :aria-label="`Jednotka ${i + 1}`"
                />
                <Button
                  v-if="canWrite"
                  icon="pi pi-times"
                  text
                  rounded
                  size="small"
                  severity="secondary"
                  :aria-label="`Odstrániť parameter ${i + 1}`"
                  @click="
                    form.params.length > 1
                      ? form.params.splice(i, 1)
                      : (form.params[0] = blankParam())
                  "
                />
                <small v-if="paramError(i, 'name')" class="err p-err">{{
                  paramError(i, "name")
                }}</small>
              </div>
            </div>
            <Button
              v-if="canWrite"
              label="Parameter"
              icon="pi pi-plus"
              text
              size="small"
              @click="form.params.push(blankParam())"
            />
            <small v-if="errors.params" class="err">{{ errors.params }}</small>
          </fieldset>

          <fieldset class="sheet pad form-section">
            <legend>Popis a poznámka</legend>
            <div class="form-grid">
              <FormField
                label="Popis produktu"
                :span="4"
                :error="errors.description"
                hint="pre ponuky a e-shop"
              >
                <Textarea
                  v-model="form.description"
                  rows="4"
                  auto-resize
                  :disabled="!canWrite"
                />
              </FormField>
              <FormField
                label="Interná poznámka"
                :span="4"
                hint="na doklady sa nedostane"
              >
                <Textarea
                  v-model="form.note"
                  rows="2"
                  auto-resize
                  :disabled="!canWrite"
                />
              </FormField>
              <label class="check">
                <Checkbox
                  v-model="form.archived"
                  binary
                  input-id="ic-archived"
                  :disabled="!canWrite"
                />
                <span>Archivovaná — neponúka sa vo faktúrach</span>
              </label>
            </div>
          </fieldset>
        </div>
      </div>

      <!-- pohyby -->
      <section
        v-if="canStock && card.tracked"
        class="sheet moves"
        aria-label="Pohyby na sklade"
      >
        <header class="moves-head">
          <h2>Pohyby</h2>
          <span class="count">{{ moves.length }}</span>
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText
              v-model="search"
              placeholder="Hľadať doklad, partnera"
              size="small"
              aria-label="Hľadať v pohyboch"
            />
          </IconField>
          <Select
            v-if="multi"
            v-model="warehouse"
            :options="warehouseOptions"
            option-label="name"
            option-value="code"
            size="small"
            aria-label="Sklad"
          />
          <div class="spacer" />
          <ExportButton
            :name="`Karta ${card.item.code || card.item.name}`"
            :columns="itemMoveColumns"
            :rows="moves"
          />
        </header>
        <div class="scroll">
          <table class="list">
            <thead>
              <tr>
                <th>Dátum</th>
                <th>Doklad</th>
                <th class="hide-m">Partner</th>
                <th v-if="multi">Sklad</th>
                <th class="num">Množstvo</th>
                <th class="num hide-m">Cena za MJ</th>
                <th class="num">Stav</th>
                <th class="num hide-m">Hodnota</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!moves.length">
                <td colspan="8" class="empty">
                  {{
                    card.moves.length
                      ? "Nič sa nenašlo."
                      : "Zatiaľ žiadny pohyb — prvú príjemku zapíšete v Sklade."
                  }}
                </td>
              </tr>
              <tr v-for="(mv, i) in moves" :key="i">
                <td class="date">{{ formatDate(mv.date) }}</td>
                <td>
                  <b>{{ mv.number }}</b>
                  <span class="kind">{{ moveLabel(mv) }}</span>
                </td>
                <td class="hide-m text">{{ mv.partner }}</td>
                <td v-if="multi">{{ mv.warehouse }}</td>
                <td class="num" :class="{ neg: Number(mv.quantity) < 0 }">
                  {{ Number(mv.quantity) > 0 ? "+" : "" }}{{ qty(mv.quantity) }}
                </td>
                <td class="num hide-m">{{ formatAmount(mv.unitCost) }}</td>
                <td class="num">{{ qty(mv.balance) }}</td>
                <td class="num hide-m">{{ formatAmount(mv.value) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.itemcard {
  padding: 14px 16px 24px;
  max-width: 1280px;
}
.missing {
  padding: 24px 0;
  color: var(--ink-soft);
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.back {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  color: var(--ink-soft);
  background: var(--sheet);
}
.title {
  min-width: 0;
}
.crumb {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.head h1 {
  font-size: var(--fs-xl);
  line-height: 1.15;
  color: var(--blue-deep);
}
.head .code {
  margin-right: 8px;
  color: var(--ink-faint);
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}
.spacer {
  flex: 1;
}

/* súhrn: hodnoty ako stĺpce v knihe, posledná suma dvojito podčiarknutá */
.ledger {
  display: flex;
  flex-wrap: wrap;
  margin: 12px 0 14px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 3px solid var(--blue-deep);
}
.ledger > div {
  flex: 1 1 140px;
  padding: 8px 14px;
  border-right: 1px solid var(--rule-soft);
}
.ledger > div:last-child {
  border-right: 0;
}
.ledger dt {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.ledger dd {
  margin: 2px 0 0;
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.ledger dd.low {
  color: var(--tone-overdue);
}
.total {
  text-decoration: underline double;
  text-underline-offset: 4px;
}

.body {
  display: grid;
  grid-template-columns: 340px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}
.col-side,
.col-main {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.pad {
  padding: 12px 14px;
}
.form-section {
  margin: 0;
  padding: 12px 14px;
}
.sheet h2 {
  font-size: var(--fs-md);
  margin-bottom: 8px;
}

.gallery {
  padding: 10px;
}
.gallery.drag {
  outline: 2px dashed var(--blue);
  outline-offset: -4px;
}
.stage {
  position: relative;
  aspect-ratio: 1;
  display: grid;
  place-items: center;
  background: var(--paper);
  border: 1px solid var(--rule-soft);
  overflow: hidden;
}
.stage img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}
.empty-img {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.empty-img .pi {
  font-size: 2.4rem;
  color: var(--rule);
}
.main-mark {
  position: absolute;
  left: 6px;
  top: 6px;
  padding: 1px 6px;
  font-size: var(--fs-xs);
  background: var(--blue-deep);
  color: #fff;
  border-radius: 2px;
}
.thumbs {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
  margin-top: 8px;
}
.thumb {
  aspect-ratio: 1;
  padding: 2px;
  border: 1px solid var(--rule);
  background: var(--paper);
  cursor: pointer;
}
.thumb.on {
  border-color: var(--blue);
  box-shadow: inset 0 0 0 1px var(--blue);
}
.thumb img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.img-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  margin-top: 8px;
}
.hidden {
  display: none;
}
.note,
.hint {
  margin-top: 6px;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.err {
  display: block;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}

.switch {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin-bottom: 10px;
  cursor: pointer;
}
.switch small {
  display: block;
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.form-grid.two {
  grid-template-columns: 1fr 1fr;
}
.wh {
  width: 100%;
  margin-top: 12px;
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.wh th {
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  border-bottom: 1px solid var(--rule);
  padding: 4px 4px;
}
.wh td {
  padding: 5px 4px;
  border-bottom: 1px solid var(--rule-soft);
}
.wh tbody tr {
  cursor: pointer;
}
.wh tbody tr.on {
  background: var(--blue-wash);
  box-shadow: inset 3px 0 0 var(--blue);
}
.wh tfoot td {
  font-weight: 600;
  border-bottom: 0;
}
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.neg {
  color: var(--tone-overdue);
}

.params {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.p-row {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) 80px 32px;
  gap: 6px;
  align-items: center;
}
.p-head {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.p-row :deep(input) {
  width: 100%;
}
.p-err {
  grid-column: 1 / -1;
}
.col-main :deep(.p-textarea) {
  font-size: var(--fs-md);
}
.check {
  grid-column: span 4;
  display: flex;
  align-items: center;
  gap: 8px;
}

.moves {
  margin-top: 14px;
}
.moves-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 10px 12px;
  border-bottom: 1px solid var(--rule);
}
.moves-head h2 {
  margin: 0;
}
.count {
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
}
.search {
  width: 260px;
}
.search :deep(input) {
  width: 100%;
}
.scroll {
  overflow-x: auto;
}
.list {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.list th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
  background: var(--paper);
}
.list td {
  padding: 5px 10px;
  border-bottom: 1px solid var(--rule-soft);
}
.list .date {
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.list .kind {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.list .empty {
  text-align: center;
  color: var(--ink-faint);
  padding: 18px;
}

@media (max-width: 980px) {
  .body {
    grid-template-columns: minmax(0, 1fr);
  }
  .col-side {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    align-items: start;
  }
}
@media (max-width: 760px) {
  .itemcard {
    padding: 10px 10px 20px;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .col-side {
    display: flex;
  }
  .hide-m {
    display: none;
  }
  .ledger > div {
    flex-basis: 45%;
    padding: 6px 10px;
  }
  .ledger dd {
    font-size: var(--fs-md);
  }
  .search {
    width: 100%;
    order: 5;
  }
  .p-row {
    grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) 56px 32px;
  }
}
</style>
