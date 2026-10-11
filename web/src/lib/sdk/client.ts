/**
 * Magpie
 * 1.0.0
 * DO NOT MODIFY - This file has been generated using oazapfts.
 * See https://www.npmjs.com/package/oazapfts
 */
import * as Oazapfts from "@oazapfts/runtime";
import * as QS from "@oazapfts/runtime/query";
export const defaults: Oazapfts.Defaults<Oazapfts.CustomHeaders> = {
    headers: {},
    baseUrl: "/"
};
const oazapfts = Oazapfts.runtime(defaults);
export const servers = {};
export type HealthResponse = {
    status: string;
};
export type ErrorItem = {
    /** Additional information about the error */
    more?: {
        [key: string]: any;
    } | {
        [key: string]: any;
    };
    /** For example, name of the parameter that caused the error */
    name: string;
    /** Human readable error message */
    reason: string;
};
export type HttpError = {
    /** Human readable error message */
    detail?: string;
    errors?: ErrorItem[] | null;
    instance?: string;
    /** HTTP status code */
    status?: number;
    /** Short title of the error */
    title?: string;
    /** URL of the error type. Can be used to lookup the error in a documentation */
    "type"?: string;
};
export type UnknownInterface = any;
/**
 * Report that the server is up
 */
export function getHealth({ accept }: {
    accept?: string;
} = {}, opts?: Oazapfts.RequestOpts) {
    return oazapfts.ok(oazapfts.fetchJson<{
        status: 200;
        data: HealthResponse;
    } | {
        status: 400;
        data: HttpError;
    } | {
        status: 500;
        data: HttpError;
    }>("/api/health", {
        ...opts,
        headers: oazapfts.mergeHeaders(opts?.headers, {
            Accept: accept
        })
    }));
}
