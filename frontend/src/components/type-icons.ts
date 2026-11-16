import {
  AlignLeft, AtSign, Binary, Braces, Calendar, CalendarClock, CircleDot, Files, Globe, Hash, LayoutList, Link2, ListOrdered, Network, Paperclip, Rows3,
  ScanSearch, Sigma, SquareFunction, Tags, Text, ToggleLeft, UserCog, UserPlus, type LucideIcon,
} from "lucide-react";
import type { FieldType } from "@/components/field-builder";

/** Each field type's icon, wherever fields are listed or picked. */
export const TYPE_ICONS: Record<FieldType, LucideIcon> = {
  string: Text, text: AlignLeft, email: AtSign, url: Globe, number: Hash, integer: Binary, boolean: ToggleLeft, enum: CircleDot, strings: Tags,
  date: Calendar, datetime: CalendarClock, reference: Link2, references: Network, lookup: ScanSearch, rollup: Sigma, file: Paperclip, files: Files,
  autonumber: ListOrdered, formula: SquareFunction, created_by: UserPlus, updated_by: UserCog, group: LayoutList, objects: Rows3, json: Braces,
};
