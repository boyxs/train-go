// 单篇文章最多标签数（对齐后端 domain.MaxTagsPerBiz）
export const MAX_TAGS = 5;

// typeahead 补全一次取回的候选上限（对齐后端 /tag/suggest limit≤10）
export const TAG_SUGGEST_LIMIT = 10;

// typeahead 输入防抖（毫秒）
export const TAG_SUGGEST_DEBOUNCE_MS = 250;
