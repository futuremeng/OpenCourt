export namespace domain {
	
	export class Citation {
	    SourceFile: string;
	    PageNum: number;
	    Snippet: string;
	    LawVersion: string;
	    LawRepealed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Citation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SourceFile = source["SourceFile"];
	        this.PageNum = source["PageNum"];
	        this.Snippet = source["Snippet"];
	        this.LawVersion = source["LawVersion"];
	        this.LawRepealed = source["LawRepealed"];
	    }
	}
	export class Conflict {
	    conflict_id: string;
	    fact_ids: string[];
	    conflict_point: string;
	    resolution_method?: string;
	    is_resolved: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Conflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conflict_id = source["conflict_id"];
	        this.fact_ids = source["fact_ids"];
	        this.conflict_point = source["conflict_point"];
	        this.resolution_method = source["resolution_method"];
	        this.is_resolved = source["is_resolved"];
	    }
	}
	export class Fact {
	    fact_id: string;
	    description: string;
	    state: string;
	    citation?: Citation;
	    related_element?: string;
	    conflict_id?: string;
	    assumption_source?: string;
	    visibility: string;
	    is_manual_confirmed: boolean;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Fact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fact_id = source["fact_id"];
	        this.description = source["description"];
	        this.state = source["state"];
	        this.citation = this.convertValues(source["citation"], Citation);
	        this.related_element = source["related_element"];
	        this.conflict_id = source["conflict_id"];
	        this.assumption_source = source["assumption_source"];
	        this.visibility = source["visibility"];
	        this.is_manual_confirmed = source["is_manual_confirmed"];
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace extract {
	
	export class Element {
	    type: string;
	    value: string;
	    source_file: string;
	    page_num: number;
	    located: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Element(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.value = source["value"];
	        this.source_file = source["source_file"];
	        this.page_num = source["page_num"];
	        this.located = source["located"];
	    }
	}

}

export namespace gate {
	
	export class Violation {
	    rule: string;
	    phrase: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Violation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.phrase = source["phrase"];
	        this.message = source["message"];
	    }
	}

}

export namespace neutral {
	
	export class Blindspot {
	    question: string;
	    supplement: string;
	
	    static createFrom(source: any = {}) {
	        return new Blindspot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.question = source["question"];
	        this.supplement = source["supplement"];
	    }
	}
	export class CandidateFact {
	    description: string;
	    snippet: string;
	    page_num: number;
	    from_opponent: boolean;
	    related_element?: string;
	
	    static createFrom(source: any = {}) {
	        return new CandidateFact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.snippet = source["snippet"];
	        this.page_num = source["page_num"];
	        this.from_opponent = source["from_opponent"];
	        this.related_element = source["related_element"];
	    }
	}
	export class GovListEntry {
	    title: string;
	    url: string;
	    cat?: string;
	    puborg?: string;
	    pcode?: string;
	
	    static createFrom(source: any = {}) {
	        return new GovListEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.url = source["url"];
	        this.cat = source["cat"];
	        this.puborg = source["puborg"];
	        this.pcode = source["pcode"];
	    }
	}
	export class GovListReport {
	    start_url: string;
	    cat: string;
	    start_page: number;
	    pages: number;
	    entries: GovListEntry[];
	    stopped_at: string;
	    out_path?: string;
	
	    static createFrom(source: any = {}) {
	        return new GovListReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start_url = source["start_url"];
	        this.cat = source["cat"];
	        this.start_page = source["start_page"];
	        this.pages = source["pages"];
	        this.entries = this.convertValues(source["entries"], GovListEntry);
	        this.stopped_at = source["stopped_at"];
	        this.out_path = source["out_path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MatchResult {
	    Covered: string[];
	    Missing: string[];
	
	    static createFrom(source: any = {}) {
	        return new MatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Covered = source["Covered"];
	        this.Missing = source["Missing"];
	    }
	}
	export class ReaderReport {
	    doc_id: string;
	    model_name: string;
	    accepted: domain.Fact[];
	    rejected: CandidateFact[];
	    elements: extract.Element[];
	
	    static createFrom(source: any = {}) {
	        return new ReaderReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.doc_id = source["doc_id"];
	        this.model_name = source["model_name"];
	        this.accepted = this.convertValues(source["accepted"], domain.Fact);
	        this.rejected = this.convertValues(source["rejected"], CandidateFact);
	        this.elements = this.convertValues(source["elements"], extract.Element);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ResearchResult {
	    url: string;
	    // Go type: time
	    fetched_at: any;
	    content_sha256: string;
	    text: string;
	    fact: domain.Fact;
	    redact_actions: redact.Action[];
	
	    static createFrom(source: any = {}) {
	        return new ResearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.fetched_at = this.convertValues(source["fetched_at"], null);
	        this.content_sha256 = source["content_sha256"];
	        this.text = source["text"];
	        this.fact = this.convertValues(source["fact"], domain.Fact);
	        this.redact_actions = this.convertValues(source["redact_actions"], redact.Action);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TimelineItem {
	    Date: string;
	    Fact: domain.Fact;
	
	    static createFrom(source: any = {}) {
	        return new TimelineItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Date = source["Date"];
	        this.Fact = this.convertValues(source["Fact"], domain.Fact);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace plugins {
	
	export class ActionCandidate {
	    action_id: string;
	    page_id: number;
	    kind: string;
	    ref: string;
	    label: string;
	    value?: string;
	    allowed: boolean;
	    rule?: string;
	    reason?: string;
	    fact_state?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionCandidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action_id = source["action_id"];
	        this.page_id = source["page_id"];
	        this.kind = source["kind"];
	        this.ref = source["ref"];
	        this.label = source["label"];
	        this.value = source["value"];
	        this.allowed = source["allowed"];
	        this.rule = source["rule"];
	        this.reason = source["reason"];
	        this.fact_state = source["fact_state"];
	    }
	}
	export class ActionReceipt {
	    action_id: string;
	    diff: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionReceipt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action_id = source["action_id"];
	        this.diff = source["diff"];
	    }
	}
	export class BrowserPage {
	    page_id: number;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.page_id = source["page_id"];
	        this.url = source["url"];
	    }
	}
	export class HumanHelpStatus {
	    status: string;
	    request_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new HumanHelpStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.request_id = source["request_id"];
	    }
	}

}

export namespace redact {
	
	export class Action {
	    rule_id: string;
	    original_hash: string;
	    strategy: string;
	    restorable: boolean;
	    // Go type: time
	    timestamp: any;
	
	    static createFrom(source: any = {}) {
	        return new Action(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule_id = source["rule_id"];
	        this.original_hash = source["original_hash"];
	        this.strategy = source["strategy"];
	        this.restorable = source["restorable"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Result {
	    text: string;
	    actions: Action[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.actions = this.convertValues(source["actions"], Action);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace storage {
	
	export class DocumentInfo {
	    doc_id: string;
	    file: string;
	    page_count: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new DocumentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.doc_id = source["doc_id"];
	        this.file = source["file"];
	        this.page_count = source["page_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

