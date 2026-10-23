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

